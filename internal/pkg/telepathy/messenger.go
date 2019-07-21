package telepathy

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	msgrSvcChannelTimeout = 10 * time.Millisecond
	msgrMsgChannelTimeout = 10 * time.Millisecond
	cmdChannelName        = "telepathy.cmd"
)

// MsgrUserProfile holds the information of a messenger user
type MsgrUserProfile struct {
	ID          string
	DisplayName string
}

// InboundMessage models a message send to Telepthy bot
type InboundMessage struct {
	FromChannel     Channel
	SourceProfile   *MsgrUserProfile
	Text            string
	IsDirectMessage bool
	Image           *Image
}

// OutboundMessage models a message send to user (through messenger)
type OutboundMessage struct {
	ToChannel Channel
	AsName    string // Sent the message as the specified user name
	Text      string // Message content
	Image     *Image // Image to be sent along with the message
}

// Messenger defines the interface of a messenger handler
// Theses interfaces are accessed only by Telepathy framework
type Messenger interface {
	plugin
	// GetSendChannel returns an input channel used to sent OutboundMessage.
	// The OutboundMessage will then be sent out through the Messenger to the specified channel.
	// Messenger Plugin MUST:
	// 1. NOT closing this channel
	// 2. Guarantee that returning the same channel every time
	// Messenger Plugin SHOULD:
	// 1. Avoid blocking sending. Sending can be cancelled if being blocked too long
	GetSendChannel() chan<- OutboundMessage
}

// MessengerManager manages messenger modules
type MessengerManager struct {
	// MessengerID -> Messenger Object
	messengers map[string]Messenger
	// MessengerID -> Message receive channel
	receivers map[string]chan InboundMessage
	// ServiceID -> Inbound Message Channel
	handlers map[string]chan<- InboundMessage
	// ServiceID -> Outbound Message Channel
	senders map[string]chan OutboundMessage
	ctx     context.Context
	cancel  context.CancelFunc
	session *Session
}

// MessengerExistsError indicates registering Messenger with a id that already exists in the list
type MessengerExistsError struct {
	ID string
}

// MessengerInvalidError indicates requesting a non-registered Messenger id
type MessengerInvalidError struct {
	ID string
}

// MessengerIllegalIDError indicates using illegal id to register a msgr handler
type MessengerIllegalIDError struct {
	ID string
}

// MsgrCtorParam is the parameter for MessengerCtor
// This is used to pass framework information to Messenger modules
type MsgrCtorParam struct {
	Session  *Session
	Config   PluginConfig
	Receiver chan<- InboundMessage
	Logger   *logrus.Entry
}

// MessengerCtor defines the signature of Messenger module constructor
// Messenger implementation need to register the constructor to the Telepathy framework
type MessengerCtor func(MsgrCtorParam) (Messenger, error)

var msgrCtors map[string]MessengerCtor
var msgLogger = logrus.WithField("module", "messengerManager")

func (e MessengerExistsError) Error() string {
	return "Messenger: " + e.ID + " has already been registered"
}

func (e MessengerInvalidError) Error() string {
	return "Messenger: " + e.ID + " does not exist"
}

func (e MessengerIllegalIDError) Error() string {
	return "Illegal messenger ID: " + e.ID
}

// RegisterMessenger registers a constructor of Messenger plugin
func RegisterMessenger(ID string, ctor MessengerCtor) error {
	logger := msgLogger.WithField("ID", ID)
	if msgrCtors == nil {
		msgrCtors = make(map[string]MessengerCtor)
	}

	if strings.Contains(ID, channelDelimiter) {
		logger.Errorf("illegal messenger ID: %s, containing: "+channelDelimiter, ID)
		return MessengerIllegalIDError{ID: ID}
	}

	if msgrCtors[ID] != nil {
		logger.Errorf("already registered: %s", ID)
		return MessengerExistsError{ID: ID}
	}

	msgrCtors[ID] = ctor
	logger.Infof("registered messenger: %s", ID)
	return nil
}

func newMessengerManager(session *Session, configTable map[string]PluginConfig) *MessengerManager {
	logger := msgLogger.WithField("phase", "new")

	manager := MessengerManager{
		messengers: make(map[string]Messenger),
		receivers:  make(map[string]chan InboundMessage),
		handlers:   make(map[string]chan<- InboundMessage),
		senders:    make(map[string]chan OutboundMessage),
		session:    session,
	}

	manager.ctx, manager.cancel = context.WithCancel(context.Background())

	// Create a channel specifically for returning root help text
	manager.createSendChannel(cmdChannelName)

	for ID, ctor := range msgrCtors {
		recv := make(chan InboundMessage)
		manager.receivers[ID] = recv
		param := MsgrCtorParam{
			Session:  session,
			Config:   configTable[ID], // Note, if config does not exists, by default we will feed a empty Config{} to the messenger constructor
			Receiver: manager.receivers[ID],
			Logger:   logrus.WithFields(logrus.Fields{"plugin": "messenger", "ID": ID}),
		}

		messenger, err := ctor(param)
		if err != nil {
			logger.WithField("ID", ID).Errorf("failed to construct: %s", err.Error())
			continue
		}

		// Constructucted plugin id must be matching the registered id
		if ID != messenger.ID() {
			logger.WithField("ID", ID).Errorf("regisered ID: %s but created as ID: %s. not constructed.",
				ID, messenger.ID())
			continue
		}

		manager.messengers[ID] = messenger

		logger.WithField("ID", ID).Info("constructed")
	}
	return &manager
}

func (m *MessengerManager) startMessengers() {
	for _, msgr := range m.messengers {
		go msgr.Start()
	}
}

func (m *MessengerManager) inboundRoutine() {
	inMsgCh := make(chan InboundMessage)
	allClosed := sync.WaitGroup{}
	allClosed.Add(len(m.receivers))

	// Aggregate inbound messages from Messengers
	for msgr, rvc := range m.receivers {
		go func() {
			for msg := range rvc {
				inMsgCh <- msg
			}
			// Receiver is closed by Messenger
			allClosed.Done()
		}()
	}

	// If all recivers are closed, close the aggregated channel
	go func() {
		allClosed.Wait()
		close(inMsgCh)
	}()

	for msg := range inMsgCh {
		m.rootMsgHandler(msg)
	}

	// If reach here, all receivers are closed and there are no more inbound messages need to be handled
	// we can now close all service and cmd channels
	for svc, handler := range m.handlers {
		close(handler)
	}
}

func (m *MessengerManager) outbooundRoutine() {
	outMsgCh := make(chan OutboundMessage)
	allClosed := sync.WaitGroup{}
	allClosed.Add(len(m.senders))
	for svc, send := range m.senders {
		go func() {
			for msg := range send {
				outMsgCh <- msg
			}
			allClosed.Done()
		}()
	}

	// If all senders are closed, close the aggregated channel
	go func() {
		allClosed.Wait()
		close(outMsgCh)
	}()

	logger := msgLogger.WithField("phase", "outbound")
	for msg := range outMsgCh {
		msgr, ok := m.messengers[msg.ToChannel.MessengerID]
		if !ok {
			logger.Errorf("invalid messenger id: %s", msg.ToChannel.MessengerID)
			continue
		}

		channel := msgr.GetSendChannel()
		if channel == nil {
			logger.WithField("ID", msg.ToChannel.MessengerID).Errorf("invalid sender channel")
			continue
		}

		ctx, cancel := context.WithTimeout(m.ctx, msgrMsgChannelTimeout)
		select {
		case msgr.GetSendChannel() <- msg:
			cancel()
		case <-ctx.Done():
			logger.WithField("ID", msg.ToChannel.MessengerID).Warnf("send timeout")
		}
	}

	// All senders are closed, close messenger channels
	for _, msgr := range m.messengers {
		close(msgr.GetSendChannel())
	}
}

func (m *MessengerManager) start() {
	allStop := sync.WaitGroup{}
	allStop.Add(2)
	go func() {
		m.inboundRoutine()
		allStop.Done()
	}()
	go func() {
		m.outbooundRoutine()
		allStop.Done()
	}()
	allStop.Wait()
	// Returns only if everything is done
}

func (m *MessengerManager) shutdown() {
	// Cancelling root context for MessengerManager, this should
	// cancel all send requests to
	// 1. Message handlers
	// 2. Command handlers
	// 3. Messenger outbound channel
	// Note that the writing channels are not closed until upstream channels are closed
	m.cancel()
}

func (m *MessengerManager) createSendChannel(ID string) chan<- OutboundMessage {
	m.senders[ID] = make(chan OutboundMessage)
	return m.senders[ID]
}

func (m *MessengerManager) addHandler(serviceID string, ch chan<- InboundMessage) {
	m.handlers[serviceID] = ch
}

func (m *MessengerManager) rootMsgHandler(message InboundMessage) {
	if isCmdMsg(message.Text) {
		m.session.Command.handleCmdMsg(m.ctx, message, m.senders[cmdChannelName])
	} else {
		for svc, handler := range m.handlers {
			go func() {
				ctx, cancel := context.WithTimeout(m.ctx, msgrSvcChannelTimeout)
				defer cancel()
				select {
				case handler <- message:
				case <-ctx.Done():
					msgLogger.WithFields(logrus.Fields{
						"phase":  "handler",
						"serice": svc}).Warnf("handler timeout")
				}
			}()
		}
	}
}

// Reply constructs an OutboundMessage targeting to the channel where the InboundMessage came from
func (im InboundMessage) Reply() OutboundMessage {
	return OutboundMessage{
		ToChannel: im.FromChannel,
	}
}
