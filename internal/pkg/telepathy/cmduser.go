package telepathy

import (
	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

func init() {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Telepathy user management",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	userCmd.AddCommand(&cobra.Command{
		Use:   "new",
		Short: "Create a new Telepathy user linked to current app. (DM only)",
		Run:   newCmdHandle,
	})
	RegisterCommand(userCmd)
}

func newCmdHandle(cmd *cobra.Command, args []string, extras ...interface{}) {
	message, ok := extras[0].(*InboundMessage)
	if !ok {
		logger := logrus.WithField("command", args)
		logger.Errorf("Invalid argument type in command handlers: %T", extras[0])
		panic("Invalid argument type in command handlers")
	}

	if !message.IsDirectMessage {
		cmd.Print("This command can only be run with Direct Messages (Whispers).")
	}
}
