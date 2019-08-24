package telepathy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func TestMessageReply(t *testing.T) {
	from := telepathy.Channel{
		MessengerID: "msg",
		ChannelID:   "ch",
	}
	msg := telepathy.InboundMessage{FromChannel: from}

	outMsg := msg.Reply()
	assert.Equal(t, from, outMsg.ToChannel)
}
