package telepathy

import (
	"context"

	"github.com/KavenC/cobra"
)

type plugin interface {
	ID() string
	Start(context.Context)
	CommandInterface() *cobra.Command
}
