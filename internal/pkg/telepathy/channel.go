package telepathy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
)

const channelDelimiter = "@"
const channelServiceID = "telepathy.channel"

type channelService struct {
	logger *logrus.Entry
}

// Channel is an abstract type for a communication session of a messenger APP
type Channel struct {
	MessengerID string
	ChannelID   string
}

func (c *channelService) ID() string {
	return channelServiceID
}

func (c *channelService) SetLogger(logger *logrus.Entry) {
	c.logger = logger
}

func (c *channelService) Start() {
	return
}

func (c *channelService) Stop() {

}

func (c *channelService) Command(_ <-chan interface{}) *argo.Action {
	cmd := &argo.Action{
		Trigger:    "channel",
		ShortDescr: "Telepathy Channel Management",
	}

	cmd.AddSubAction(argo.Action{
		Trigger:    "info",
		ShortDescr: "Show the name of current channel",
		Do:         cmdChannelInfo,
	})

	return cmd
}

func cmdChannelInfo(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(CmdExtraArgs)
	if !ok {
		logrus.WithField("module", "channel").Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}
	state.OutputStr.WriteString(extraArgs.Message.FromChannel.Name())
	return nil
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
