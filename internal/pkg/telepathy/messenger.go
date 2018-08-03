package telepathy

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
)

// MsgrUserProfile holds the information of a messenger user
type MsgrUserProfile struct {
	ID          string
	DisplayName string
}

// InboundMessage models a message send to Telepthy bot
type InboundMessage struct {
	Messenger     Messenger
	SourceProfile *MsgrUserProfile
	SourceID      string
	Text          string
}

// OutboundMessage models a message send to Client (through messenger)
type OutboundMessage struct {
	TargetID string
	Text     string
}

// Messenger defines the interface of a messenger handler
type Messenger interface {
	name() string
	init() error
	start(context.Context)
	send(*OutboundMessage)
}

var messengerList map[string]Messenger

// RegisterMessenger registers a Messenger handler
func RegisterMessenger(messenger Messenger) {
	logger := logrus.WithField("messenger", messenger.name())
	logger.Info("Registering Messenger.")
	if messengerList == nil {
		messengerList = make(map[string]Messenger)
	}

	if messengerList[messenger.name()] != nil {
		panic("Messenger: " + messenger.name() + " already exists.")
	}

	messengerList[messenger.name()] = messenger
}

// HandleInboundMessage handles incoming message from messengers
func HandleInboundMessage(message *InboundMessage) {
	if isCmdMsg(message.Text) {
		// Got a command message
		// Parse it with command interface
		args := getCmdFromMsg(message.Text)
		logger := logrus.WithFields(logrus.Fields{
			"messenger": message.Messenger.name(),
			"source":    message.SourceID,
			"args":      args})
		logger.Info("Got command message.")

		rootCmd.SetArgs(args)
		var buffer strings.Builder
		rootCmd.SetOutput(&buffer)
		rootCmd.Execute()

		// If there is some stirng output, forward it back to user
		if buffer.Len() > 0 {
			replyMsg := &OutboundMessage{TargetID: message.SourceID, Text: buffer.String()}
			logger = logrus.WithFields(logrus.Fields{
				"messenger": message.Messenger.name(),
				"target":    replyMsg.TargetID,
			})
			logger.Info("Send command reply")
			message.Messenger.send(replyMsg)
		}
	}
}
