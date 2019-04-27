package telepathy

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
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

// OutboundMessage models a message send to Client (through messenger)
type OutboundMessage struct {
	TargetID string
	Text     string
	Image    *Image
}

// GlobalMessenger defines global interfaces of a messenger handler
// These interfaces will be opened to external modules such as other Messenger handlers
type GlobalMessenger interface {
	Send(*OutboundMessage) // Must be go-routine safe
}

// MessengerPlugin has to be embedded for Messenger plugins
type MessengerPlugin struct{}

// Messenger defines the interface of a messenger handler
// Theses interfaces are accessed only by Telepathy framework
type Messenger interface {
	plugin
	GlobalMessenger
}

// MessageManager manages messenger modules
type MessageManager struct {
	messengers map[string]Messenger
	msgHandler []InboundMsgHandler
	session    *Session
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
	Session    *Session
	Config     PluginConfig
	MsgHandler InboundMsgHandler
	Logger     *logrus.Entry
}

// InboundMsgHandler defines the signature of unified inbound message handler
// Every Messenger implementation should call this function for all received messages
type InboundMsgHandler func(context.Context, InboundMessage)

// MessengerCtor defines the signature of Messenger module constructor
// Messenger implementation need to register the constructor to the Telepathy framework
type MessengerCtor func(*MsgrCtorParam) (Messenger, error)

var msgrCtors map[string]MessengerCtor

func (e MessengerExistsError) Error() string {
	return "Messenger: " + e.ID + " has already been registered"
}

func (e MessengerInvalidError) Error() string {
	return "Messenger: " + e.ID + " does not exist"
}

func (e MessengerIllegalIDError) Error() string {
	return "Illegal messenger ID: " + e.ID
}

// CommandInterface returns the command interface of a Messenger
// The stub here makes it optional to implement this for Messenger plugins
func (m *MessengerPlugin) CommandInterface() *argo.Action {
	return nil
}

// RegisterMessenger registers a Messenger handler
func RegisterMessenger(ID string, ctor MessengerCtor) error {
	logger := logrus.WithField("module", "messageManager").WithField("messenger", ID)
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

func newMessageManager(session *Session, configTable map[string]PluginConfig) *MessageManager {
	logger := logrus.WithField("module", "messageManager")
	manager := MessageManager{
		messengers: make(map[string]Messenger),
		session:    session,
	}
	param := MsgrCtorParam{
		Session:    session,
		MsgHandler: manager.rootMsgHandler,
	}

	for ID, ctor := range msgrCtors {
		param := param
		param.Logger = logrus.WithField("messenger", ID)
		config, configExists := configTable[ID]
		if !configExists {
			logger.WithField("messenger", ID).Warnf("config for %s does not exist", ID)
		}
		param.Config = config
		messenger, err := ctor(&param)
		if err != nil {
			logger.WithField("messenger", ID).Errorf("failed to construct: %s", err.Error())
			continue
		}

		// Constructucted plugin id must be matching the registered id
		if ID != messenger.ID() {
			logger.WithField("messenger", ID).Errorf("regisered ID: %s but created as ID: %s. not constructed.",
				ID, messenger.ID())
			continue
		}

		manager.messengers[ID] = messenger

		// Register command interfaces
		if cmd := messenger.CommandInterface(); cmd != nil {
			manager.session.Command.RegisterCommand(cmd)
		}

		logger.WithField("messenger", ID).Infof("constructed messenger: %s", ID)
	}
	return &manager
}

// Messenger gets a registered messenger handler with ID
func (m *MessageManager) Messenger(ID string) (GlobalMessenger, error) {
	msg := m.messengers[ID]
	if msg == nil {
		return nil, MessengerInvalidError{ID: ID}
	}
	return msg, nil
}

// RegisterMessageHandler register a InboundMsgHandler
// The callback will be called when receiving messages from any Messenger
func (m *MessageManager) RegisterMessageHandler(handler InboundMsgHandler) {
	m.msgHandler = append(m.msgHandler, handler)
}

func (m *MessageManager) rootMsgHandler(ctx context.Context, message InboundMessage) {
	if isCmdMsg(message.Text) {
		go m.session.Command.handleCmdMsg(ctx, &message)
	} else {
		for _, handler := range m.msgHandler {
			go handler(ctx, message)
		}
	}
}
