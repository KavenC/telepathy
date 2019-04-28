package telepathy

import (
	"context"

	"gitlab.com/kavenc/argo"
)

// PluginConfig is a configuration map which passes to registered
// constructor of each plugin
// The content will be defined by Plugin
type PluginConfig map[string]interface{}

type plugin interface {
	ID() string
	Start(context.Context)
	CommandInterface() *argo.Action
}
