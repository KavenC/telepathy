package telepathy

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gitlab.com/kavenc/argo"

	"github.com/stretchr/testify/assert"
)

func TestCmdTrigger(t *testing.T) {
	assert := assert.New(t)
	cmdCh := make(chan InboundMessage)
	cmdMgr := newCmdManager("test", 3, time.Second, cmdCh)

	fromCh := Channel{
		MessengerID: "msg",
		ChannelID:   "ch",
	}
	cmdMsg := InboundMessage{
		FromChannel: fromCh,
		Text:        "test subcmd act",
	}

	subCmd := &argo.Action{Trigger: "subcmd"}
	subCmd.AddSubAction(argo.Action{
		Trigger: "act",
		Do: func(state *argo.State, extras ...interface{}) error {
			extraArgs, ok := extras[0].(CmdExtraArgs)
			assert.True(ok)
			assert.Equal(cmdMsg, extraArgs.Message)
			assert.Equal("test", extraArgs.Prefix)
			fmt.Fprint(&state.OutputStr, "output test")
			return nil
		},
	})

	cmdMgr.attachCommandInterface(subCmd)
	go cmdMgr.start(context.Background())

	cmdCh <- cmdMsg
	outMsg := <-cmdMgr.msgOut
	assert.Equal("output test", outMsg.Text)
	assert.Equal(fromCh, outMsg.ToChannel)

	close(cmdCh)
	<-cmdMgr.done
}

func TestCmdIsCmdMsg(t *testing.T) {
	assert := assert.New(t)
	cmdCh := make(chan InboundMessage)
	cmdMgr := newCmdManager("test", 1, time.Second, cmdCh)

	assert.True(cmdMgr.isCmdMsg("test abc"))
	assert.True(cmdMgr.isCmdMsg("test      abc"))
	assert.False(cmdMgr.isCmdMsg("test"))
	assert.False(cmdMgr.isCmdMsg("  test"))
	assert.False(cmdMgr.isCmdMsg("tes"))
	assert.False(cmdMgr.isCmdMsg("test	")) // Tab

	go cmdMgr.start(context.Background())
	close(cmdCh)
	<-cmdMgr.done
}

func TestCmdTimeout(t *testing.T) {
	assert := assert.New(t)
	cmdCh := make(chan InboundMessage)
	cmdMgr := newCmdManager("test", 3, time.Second, cmdCh)

	fromCh := Channel{
		MessengerID: "msg",
		ChannelID:   "ch",
	}
	cmdMsg := InboundMessage{
		FromChannel: fromCh,
		Text:        "test subcmd act",
	}

	subCmd := &argo.Action{Trigger: "subcmd"}
	subCmd.AddSubAction(argo.Action{
		Trigger: "act",
		Do: func(state *argo.State, extras ...interface{}) error {
			start := time.Now()
			extraArgs, _ := extras[0].(CmdExtraArgs)
			<-extraArgs.Ctx.Done()
			assert.WithinDuration(start, time.Now(), 1100*time.Millisecond)
			return nil
		},
	})

	cmdMgr.attachCommandInterface(subCmd)
	go cmdMgr.start(context.Background())

	cmdCh <- cmdMsg
	assert.Equal(0, len(cmdMgr.msgOut))

	close(cmdCh)
	<-cmdMgr.done
}

func TestCmdReturnError(t *testing.T) {
	assert := assert.New(t)
	cmdCh := make(chan InboundMessage)
	cmdMgr := newCmdManager("test", 3, time.Second, cmdCh)

	fromCh := Channel{
		MessengerID: "msg",
		ChannelID:   "ch",
	}
	cmdMsg := InboundMessage{
		FromChannel: fromCh,
		Text:        "test subcmd act",
	}

	subCmd := &argo.Action{Trigger: "subcmd"}
	subCmd.AddSubAction(argo.Action{
		Trigger: "act",
		Do: func(state *argo.State, extras ...interface{}) error {
			return errors.New("error")
		},
	})

	cmdMgr.attachCommandInterface(subCmd)
	go cmdMgr.start(context.Background())

	cmdCh <- cmdMsg
	outMsg := <-cmdMgr.msgOut

	assert.Contains(outMsg.Text, "Internal Error")

	close(cmdCh)
	<-cmdMgr.done
}

func TestCmdDuplicated(t *testing.T) {
	assert := assert.New(t)
	cmdCh := make(chan InboundMessage)
	cmdMgr := newCmdManager("test", 3, time.Second, cmdCh)

	subCmd := &argo.Action{Trigger: "subcmd"}
	dupCmd := &argo.Action{Trigger: "subcmd"}

	err := cmdMgr.attachCommandInterface(subCmd)
	assert.NoError(err)
	err = cmdMgr.attachCommandInterface(dupCmd)
	assert.Error(err)
	_, ok := err.(argo.DuplicatedSubActionError)
	assert.True(ok)

	go cmdMgr.start(context.Background())
	close(cmdCh)
	<-cmdMgr.done
}
