package telepathy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

const channelDelimiter = "@"
const channelServiceID = "telepathy.channel"

type channelService struct {
	ServicePlugin
}

// Channel is an abstract type for a communication session of a messenger APP
type Channel struct {
	MessengerID string
	ChannelID   string
}

func newChannelService(*ServiceCtorParam) (Service, error) {
	return &channelService{}, nil
}

func (c *channelService) Start(context context.Context) {
	return
}

func (c *channelService) ID() string {
	return channelServiceID
}

func (c *channelService) CommandInterface() *cobra.Command {
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

	return cmd
}

func cmdChannelName(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs, ok := extras[0].(CmdExtraArgs)
	if !ok {
		logrus.WithField("module", "channel").Errorf("failed to parse extraArgs: %T", extras[0])
		cmd.Print("Internal error. Command failed.")
		return
	}
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
