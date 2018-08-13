package telepathy

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// Channel is an abstract type for a communication session of a messenger APP
type Channel struct {
	MessengerID string
	ChannelID   string
}

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

// GetMessenger gets a registered messenger handler with ID
func GetMessenger(ID string) (Messenger, error) {
	msg := messengerList[ID]
	if msg == nil {
		return nil, errors.New("Invalid Messenger ID: " + ID)
	}
	return msg, nil
}

func msgrHandleForwarding(ctx context.Context, message *InboundMessage) {
	fwd := GetForwarding()
	toChList := fwd.GetForwardingTo(message.FromChannel)
	if toChList != nil {
		text := fmt.Sprintf("[%s] %s:\n%s",
			message.FromChannel.MessengerID,
			message.SourceProfile.DisplayName,
			message.Text)
		for toCh := range toChList {
			outMsg := &OutboundMessage{
				TargetID: toCh.ChannelID,
				Text:     text,
			}
			msgr, _ := GetMessenger(toCh.MessengerID)
			msgr.send(outMsg)
		}
	}
}

// HandleInboundMessage handles incoming message from messengers
func HandleInboundMessage(ctx context.Context, message *InboundMessage) {
	if isCmdMsg(message.Text) {
		// Got a command message
		// Parse it with command interface
		args := getCmdFromMsg(message.Text)
		logger := logrus.WithFields(logrus.Fields{
			"messenger": message.FromChannel.MessengerID,
			"source":    message.FromChannel.ChannelID,
			"args":      args})
		logger.Info("Got command message.")

		rootCmd.SetArgs(args)
		var buffer strings.Builder
		rootCmd.SetOutput(&buffer)
		rootCmd.Execute(ctx, message)

		// If there is some stirng output, forward it back to user
		if buffer.Len() > 0 {
			replyMsg := &OutboundMessage{
				TargetID: message.FromChannel.ChannelID,
				Text:     buffer.String(),
			}
			logger = logrus.WithFields(logrus.Fields{
				"messenger": message.FromChannel.MessengerID,
				"target":    replyMsg.TargetID,
			})
			logger.Info("Send command reply")

			msg, _ := GetMessenger(message.FromChannel.MessengerID)
			msg.send(replyMsg)
		}
	} else {
		msgrHandleForwarding(ctx, message)
	}
}

// GetName returns a formated name of a Channel object
func (ch *Channel) GetName() string {
	return fmt.Sprintf("%s(%s)", ch.MessengerID, ch.ChannelID)
}
