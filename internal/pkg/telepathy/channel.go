package telepathy

import (
	"fmt"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

// Channel is an abstract type for a communication session of a messenger APP
type Channel struct {
	MessengerID string
	ChannelID   string
}

func init() {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Telepathy channel management",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "name",
		Short: "Show the name of current channel.",
		Run:   cmdChannelName,
	})

	RegisterCommand(cmd)
}

func cmdChannelName(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := CommandParseExtraArgs(
		logrus.WithField("command", cmd.CommandPath),
		extras...)

	cmd.Print(extraArgs.Message.FromChannel.Name())
}

// Name returns a formated name of a Channel object
func (ch *Channel) Name() string {
	return fmt.Sprintf("%s(%s)", ch.MessengerID, ch.ChannelID)
}