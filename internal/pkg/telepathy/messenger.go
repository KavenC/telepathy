package telepathy

import (
	"context"

	"github.com/sirupsen/logrus"
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
}

// OutboundMessage models a message send to Client (through messenger)
type OutboundMessage struct {
	TargetID string
	Text     string
}

// GlobalMessenger defines global interfaces of a messenger handler
// These interfaces will be opened to external modules such as other Messenger handlers
type GlobalMessenger interface {
	Name() string
	Send(*OutboundMessage)
}

// Messenger defines the interface of a messenger handler
// Theses interfaces are accessed only by Telepathy framework
type Messenger interface {
	Start(context.Context)
	GlobalMessenger
}

// MessengerManager manages messenger modules
type MessengerManager struct {
	messengers map[string]Messenger
	session    *Session
}

// MessengerExistsError indicates registering Messenger with a name that already exists in the list
type MessengerExistsError struct {
	Name string
}

// MessengerInvalidError indicates requesting a non-registered Messenger name
type MessengerInvalidError struct {
	Name string
}

// MsgrCtorParam is the parameter for MessengerCtor
// This is used to pass framework information to Messenger modules
type MsgrCtorParam struct {
	Session    *Session
	MsgHandler InboundMsgHandler
	Logger     *logrus.Entry
}

// InboundMsgHandler defines the signature of unified inbound message handler
// Every Messenger implementation should call this function for all received messages
type InboundMsgHandler func(context.Context, *Session, InboundMessage)

// MessengerCtor defines the signature of Messenger module constructor
// Messenger implementation need to register the constructor to the Telepathy framework
type MessengerCtor func(*MsgrCtorParam) (Messenger, error)

var msgrCtors map[string]MessengerCtor
var msgHandler []InboundMsgHandler

func (e MessengerExistsError) Error() string {
	return "Messenger: " + e.Name + " has already been registered"
}

func (e MessengerInvalidError) Error() string {
	return "Messenger: " + e.Name + " does not exist"
}

// RegisterMessenger registers a Messenger handler
func RegisterMessenger(ID string, ctor MessengerCtor) error {
	logger := logrus.WithField("messenger", ID)
	logger.Info("Registering Messenger")
	if msgrCtors == nil {
		msgrCtors = make(map[string]MessengerCtor)
	}

	if msgrCtors[ID] != nil {
		return MessengerExistsError{Name: ID}
	}

	msgrCtors[ID] = ctor
	return nil
}

// RegisterMessageHandler register a InboundMsgHandler
// The callback will be called when receiving messages from any Messenger
func RegisterMessageHandler(handler InboundMsgHandler) {
	msgHandler = append(msgHandler, handler)
}

func newMessengerManager(session *Session) *MessengerManager {
	manager := MessengerManager{
		messengers: make(map[string]Messenger),
		session:    session,
	}
	param := MsgrCtorParam{
		Session:    session,
		MsgHandler: rootMsgHandler,
	}
	var err error

	for ID, ctor := range msgrCtors {
		param := param
		param.Logger = logrus.WithField("messenger", ID)
		manager.messengers[ID], err = ctor(&param)
		if err != nil {
			logger := logrus.WithField("module", "messenger")
			logger.Error(err.Error())
		}
	}

	return &manager
}

// Messenger gets a registered messenger handler with ID
func (m *MessengerManager) Messenger(ID string) (GlobalMessenger, error) {
	msg := m.messengers[ID]
	if msg == nil {
		return nil, MessengerInvalidError{Name: ID}
	}
	return msg, nil
}

func rootMsgHandler(ctx context.Context, session *Session, message InboundMessage) {
	if isCmdMsg(message.Text) {
		logger := logrus.WithFields(logrus.Fields{
			"messenger": message.FromChannel.MessengerID,
			"source":    message.FromChannel.ChannelID,
			"text":      message.Text})
		logger.Info("Got command message")
		handleCmdMsg(ctx, session, &message)
	} else {
		for _, handler := range msgHandler {
			handler(ctx, session, message)
		}
	}
}
