package telepathy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

const channelDelimiter = "@"

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
	extraArgs := NewCmdExtraArgs(extras...)
	cmd.Print(extraArgs.Message.FromChannel.Name())
}

// Name returns a formated name of a Channel object
func (ch *Channel) Name() string {
	ret := fmt.Sprintf("%s%s%s", ch.MessengerID, channelDelimiter, ch.ChannelID)
	if strings.Contains(ch.ChannelID, channelDelimiter) {
		logrus.WithField("module", "channel").Warn("channel id contains delimeter: " + ret)
	}
	return ret
}

// NewChannel creates a channel object from channel name
func NewChannel(channelName string) *Channel {
	s := strings.Split(channelName, channelDelimiter)
	return &Channel{
		MessengerID: s[0],
		ChannelID:   strings.Join(s[1:], "")}
}

// JSON returns JSON representation of Channel
func (ch *Channel) JSON() string {
	str, _ := json.Marshal(ch)
	return string(str)
}
