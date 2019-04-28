package fwd

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

var logger = logrus.WithField("module", "fwd")

const funcKey = "fwd"

// PublicError is the type of error that is intended to feedback to user
// The error message will be returned to user
type PublicError struct {
	Msg string
}

// InternalError is the type of error that should not be disclosed to user
type InternalError struct {
	Msg string
}

// TerminatedError indicates system has be interanlly terminated
type TerminatedError struct {
	Msg string
}

func (e PublicError) Error() string {
	return e.Msg
}

func (e InternalError) Error() string {
	return e.Msg
}

func (e TerminatedError) Error() string {
	return "Terminated: " + e.Msg
}

func (m *forwardingManager) CommandInterface() *argo.Action {
	cmd := &argo.Action{
		Trigger:    funcKey,
		ShortDescr: "Cross-app Message Forwarding",
	}

	cmd.AddSubAction(argo.Action{
		Trigger:    "2way",
		ShortDescr: "Create two-way channel forwarding",
		LongDescr:  "Setup message forwarding between 2 channels",
		MinConsume: 2,
		ArgNames:   []string{"channel-alias-1", "channel-alias-2"},
		Do:         m.createTwoWay,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "1way",
		ShortDescr: "Create one-way channel forwarding",
		LongDescr:  "Setup message forwarding from 1 channel to another",
		MinConsume: 2,
		ArgNames:   []string{"channel-alias-1", "channel-alias-2"},
		Do:         m.createOneWay,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "info",
		ShortDescr: "Show message forwarding info",
		Do:         m.info,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "del-from",
		MinConsume: 1,
		MaxConsume: -1,
		ArgNames:   []string{"channel-id", "channel-id"},
		ShortDescr: "Stop receiving forwarded messages",
		Do:         m.delFrom,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "del-to",
		MinConsume: 1,
		MaxConsume: -1,
		ArgNames:   []string{"channel-id", "channel-id"},
		ShortDescr: "Stop forwarding messages",
		Do:         m.delTo,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "set",
		ShortDescr: "Used for identify channel",
		ArgNames:   []string{"hash"},
		MinConsume: 1,
		Do:         m.set,
	})

	return cmd
}

func (m *forwardingManager) createTwoWay(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	if !telepathy.CommandEnsureDM(state, extraArgs) {
		return nil
	}

	state.OutputStr.WriteString("Setup two-way channel forwarding (1st Channel <-> 2nd Channel)\n")
	m.setupFwd(state, Session{Cmd: twoWay}, extraArgs)
	return nil
}

func (m *forwardingManager) createOneWay(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	if !telepathy.CommandEnsureDM(state, extraArgs) {
		return nil
	}

	state.OutputStr.WriteString("Setup one-way channel forwarding (1st Channel -> 2nd Channel)\n")
	m.setupFwd(state, Session{Cmd: oneWay}, extraArgs)
	return nil
}

func (m *forwardingManager) info(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	toChList := m.forwardingTo(extraArgs.Message.FromChannel)
	if toChList != nil {
		state.OutputStr.WriteString("= Messages are forwarding to:")
		for toCh := range toChList {
			fmt.Fprintf(&state.OutputStr, "\n%s", toCh.Name())
		}
	}

	fromChList := m.forwardingFrom(extraArgs.Message.FromChannel)
	if fromChList != nil {
		state.OutputStr.WriteString("\n\n= Receiving forwarded messages from:")
		for fromCh := range fromChList {
			fmt.Fprintf(&state.OutputStr, "\n%s", fromCh.Name())
		}
	}

	if toChList == nil && fromChList == nil {
		state.OutputStr.WriteString("This channel is not in any forwarding pairs.")
	}

	return nil
}

func (m *forwardingManager) set(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	args := state.Args()
	key := args[0]

	// Set key in redis
	redisRet := make(chan interface{})
	m.session.Redis.PushRequest(&telepathy.RedisRequest{
		Action: m.setKeyProcess(key, extraArgs.Message.FromChannel),
		Return: redisRet,
	})

	// Wait for reply
	select {
	case <-extraArgs.Ctx.Done():
		logger.Warn("Terminated")
	case reply := <-redisRet:
		replyStr, _ := reply.(string)
		if replyStr != "" {
			state.OutputStr.WriteString(replyStr)
		}
	}

	return nil
}

func (m *forwardingManager) delFrom(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}
	thisCh := extraArgs.Message.FromChannel

	change := false
	for _, fromChName := range state.Args() {
		fromCh := telepathy.NewChannel(fromChName)
		if !m.table.DelChannel(*fromCh, thisCh) {
			fmt.Fprintf(&state.OutputStr, "Message forwarding from: %s does not exist\n", fromChName)
			continue
		}
		fmt.Fprintf(&state.OutputStr, "Stop receiving messages from: %s\n", fromChName)
		messenger, _ := m.session.Message.Messenger(fromCh.MessengerID)
		msg := telepathy.OutboundMessage{
			TargetID: fromCh.ChannelID,
			Text:     fmt.Sprintf("Message forwarding to: %s has been stopped\n", thisCh.Name()),
		}
		messenger.Send(&msg)
		change = true
	}

	if change {
		m.writeToDB()
	}

	return nil
}

func (m *forwardingManager) delTo(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}
	thisCh := extraArgs.Message.FromChannel

	change := false
	for _, toChName := range state.Args() {
		toCh := telepathy.NewChannel(toChName)
		if !m.table.DelChannel(thisCh, *toCh) {
			fmt.Fprintf(&state.OutputStr, "Message forwarding to: %s doesnot exist\n", toChName)
			continue
		}
		fmt.Fprintf(&state.OutputStr, "Stop forwarding messages to: %s\n", toChName)
		messenger, _ := m.session.Message.Messenger(toCh.MessengerID)
		msg := telepathy.OutboundMessage{
			TargetID: toCh.ChannelID,
			Text:     fmt.Sprintf("Message forwarding from: %s has been stopped\n", thisCh.Name()),
		}
		messenger.Send(&msg)
		change = true
	}

	if change {
		m.writeToDB()
	}

	return nil
}
