package telepathy

import (
	"context"

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
	if len(extras) != 2 {
		logger := logrus.WithField("command", args)
		logger.Errorf("Insufficient arguments in command handler.")
		panic("Insufficient arguments in command handler.")
	}

	_, ctxok := extras[0].(context.Context)
	message, msgok := extras[1].(*InboundMessage)
	if !ctxok || !msgok {
		logger := logrus.WithField("command", args)
		logger.Errorf("Invalid argument type in command handler: %T, %T", extras[0], extras[1])
		panic("Invalid argument type in command handlers")
	}

	if !message.IsDirectMessage {
		cmd.Print("This command can only be run with Direct Messages (Whispers).")
	}
}
