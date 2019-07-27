package telepathy

import (
	"net/url"

	"gitlab.com/kavenc/argo"

	"github.com/sirupsen/logrus"
)

// Plugin defines the functions that need to be implemented for all plugins
// Plugins may optionally implement other functions by implement intefaces below
type Plugin interface {
	// Id returns the unique id for the plugin
	ID() string

	// SetLogger will be called in init stage to provide logger for the plugin
	SetLogger(*logrus.Entry)

	// Start is the main routine of the plugin
	// this function only returns when the plugin is terminated
	Start()

	// Stop triggers termination of the plugin
	Stop()
}

// PluginMessenger defines the necessary functions for a messenger plugin
// A messenger plugin which serves as the interface for messenger app must implement PluginMessenger
type PluginMessenger interface {
	// InMsgChannel should provide the channel used to get
	// all inbound messages received by the messenger plugin
	InMsgChannel() <-chan InboundMessage

	// AttachOutMsgChannel is used to attach outbound message
	// that should be sent out by the messenger plugin
	AttachOutMsgChannel(<-chan OutboundMessage)
}

// PluginCommandHandler defines the necessary functions if a plugin implements command intefaces
// The input parameter channel will be closed once the command parser is terminated
// and no more command will be triggered
type PluginCommandHandler interface {
	Command(<-chan interface{}) *argo.Action
}

// PluginWebhookHandler defines the necessary functions if a plugin is handling webhook
type PluginWebhookHandler interface {
	Webhook() map[string]HTTPHandler
	SetWebhookURL(map[string]*url.URL)
}

// PluginMsgConsumer defines the necesaary functions if a plugin handles inbound messages
type PluginMsgConsumer interface {
	AttachInMsgChannel(<-chan InboundMessage)
}

// PluginMsgProducer defeins necessary functions if a plugin would send out messages
type PluginMsgProducer interface {
	OutMsgChannel() <-chan OutboundMessage
}

// PluginDatabaseUser defines necessary functions if a plugin accesses database
type PluginDatabaseUser interface {
	DBRequestChannel() <-chan DatabaseRequest
}

// PluginRedisUser defines necessary functions ifa plugin uses Redis
type PluginRedisUser interface {
	RedisRequestChannel() <-chan RedisRequest
}
