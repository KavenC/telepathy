package telepathy

import (
	"context"
	"encoding/json"
	"testing"

	"gitlab.com/kavenc/argo"

	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
)

func TestChannelStruct(t *testing.T) {
	assert := assert.New(t)
	channel := Channel{
		MessengerID: "msg",
		ChannelID:   "ch",
	}

	newChannel := NewChannel(channel.Name())
	assert.Equal(channel, *newChannel)

	jsonChannel := Channel{}
	json.Unmarshal([]byte(channel.JSON()), &jsonChannel)

	assert.Equal(channel, jsonChannel)
}

func TestChannelPluginInterface(t *testing.T) {
	assert := assert.New(t)
	svc := &channelService{}

	var plugin Plugin
	plugin = svc

	assert.Equal(plugin.ID(), "telepathy.channel")
	logger := logrus.WithField("test", "test")
	plugin.SetLogger(logger)
	assert.Equal(svc.logger, logger)
}

func TestChannelCommand(t *testing.T) {
	assert := assert.New(t)
	svc := &channelService{}

	var cmd PluginCommandHandler
	cmd = svc

	action := cmd.Command(make(chan interface{}))
	assert.NoError(action.Finalize())
	state := argo.State{}
	ctx := context.Background()
	from := Channel{
		ChannelID:   "ch",
		MessengerID: "msg",
	}
	extArgs := CmdExtraArgs{
		Ctx: ctx,
		Message: InboundMessage{
			FromChannel: from,
		},
	}
	assert.NoError(action.Parse(&state, []string{"channel", "info"}, extArgs))
	assert.Contains(state.OutputStr.String(), from.Name())
}
