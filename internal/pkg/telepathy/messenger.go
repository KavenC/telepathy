package telepathy

import (
	"github.com/sirupsen/logrus"
)

// MsgrUserProfile holds the information of a messenger user
type MsgrUserProfile struct {
	ID          string
	DisplayName string
}

// Message is a general type of message that will be used in telepathy
type Message struct {
	Messenger     Messenger
	SourceProfile *MsgrUserProfile
	ReplyID       string
	Text          string
}

// Messenger defines the interface of a messenger handler
type Messenger interface {
	name() string
	start()
}

var messengerList map[string]Messenger

// RegisterMessenger registers a Messenger handler
func RegisterMessenger(messenger Messenger) {
	logrus.Info("Registering Messenger: " + messenger.name())
	if messengerList == nil {
		messengerList = make(map[string]Messenger)
	}

	if messengerList[messenger.name()] != nil {
		panic("Messenger: " + messenger.name() + " already exists.")
	}

	messengerList[messenger.name()] = messenger
}

// HandleMessage handles incoming message from messengers
func HandleMessage(message *Message) {
	logrus.Info("Message: name=" + message.Messenger.name() +
		" from=" + message.SourceProfile.DisplayName +
		" text=" + message.Text +
		" reply=" + message.ReplyID)
}
