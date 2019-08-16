package fwd

import (
	"errors"
	"fmt"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// Command implements telepathy.PluginCommandHandler
func (m *Service) Command(done <-chan interface{}) *argo.Action {
	m.cmdDone = done
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

func (m *Service) createTwoWay(state *argo.State, extras ...interface{}) error {
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

func (m *Service) createOneWay(state *argo.State, extras ...interface{}) error {
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

func (m *Service) info(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	toChList := m.table.getTo(extraArgs.Message.FromChannel)
	if toChList != nil {
		state.OutputStr.WriteString("= Messages are forwarding to:")
		for toCh, alias := range toChList {
			fmt.Fprintf(&state.OutputStr, "\n%s (%s)", alias.DstAlias, toCh.MessengerID)
		}
	}

	fromChList := m.table.getFrom(extraArgs.Message.FromChannel)
	if fromChList != nil {
		state.OutputStr.WriteString("\n\n= Receiving forwarded messages from:")
		for fromCh, alias := range fromChList {
			fmt.Fprintf(&state.OutputStr, "\n%s (%s)", alias.SrcAlias, fromCh.MessengerID)
		}
	}

	if toChList == nil && fromChList == nil {
		state.OutputStr.WriteString("This channel is not in any forwarding pairs.")
	}

	return nil
}

func (m *Service) set(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}

	args := state.Args()
	key := args[0]

	reply, err := m.setKeyProcess(key, extraArgs.Message.FromChannel)
	if err != nil {
		return err
	}
	state.OutputStr.WriteString(reply)
	return nil
}

func sliceUniqify(input []string) []string {
	uniqueMap := make(map[string]bool)
	output := make([]string, 0, len(input))
	for _, value := range input {
		_, ok := uniqueMap[value]
		if ok {
			continue
		}
		uniqueMap[value] = true
		output = append(output, value)
	}
	return output
}

func (m *Service) delFrom(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}
	thisCh := extraArgs.Message.FromChannel
	uniqueArgs := sliceUniqify(state.Args())

	change := false
	fromList := m.table.getFrom(thisCh)
	aliasMap := make(map[string]telepathy.Channel)
	for fromCh, alias := range fromList {
		aliasMap[alias.SrcAlias] = fromCh
	}

	for _, fromChName := range uniqueArgs {
		fromCh, ok := aliasMap[fromChName]
		toChName := fromList[fromCh].DstAlias
		if ok && <-m.table.delete(fromCh, thisCh) {
			fmt.Fprintf(&state.OutputStr, "Stop receiving messages from: %s\n", fromChName)
			msg := telepathy.OutboundMessage{
				ToChannel: fromCh,
				Text:      fmt.Sprintf("Message forwarding to: %s has been stopped\n", toChName),
			}
			m.outMsg <- msg
			change = true
		} else {
			fmt.Fprintf(&state.OutputStr, "Message forwarding from: %s does not exist\n", fromChName)
		}
	}

	if change {
		m.writeToDB()
	}

	return nil
}

func (m *Service) delTo(state *argo.State, extras ...interface{}) error {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		return errors.New("failed to parse extraArgs")
	}
	thisCh := extraArgs.Message.FromChannel
	uniqueArgs := sliceUniqify(state.Args())

	change := false
	toList := m.table.getTo(thisCh)
	aliasMap := make(map[string]telepathy.Channel)
	for toCh, alias := range toList {
		aliasMap[alias.DstAlias] = toCh
	}

	for _, toChName := range uniqueArgs {
		toCh, ok := aliasMap[toChName]
		fromChName := toList[toCh].SrcAlias
		if ok && <-m.table.delete(thisCh, toCh) {
			fmt.Fprintf(&state.OutputStr, "Stop forwarding messages to: %s\n", toChName)
			msg := telepathy.OutboundMessage{
				ToChannel: toCh,
				Text:      fmt.Sprintf("Message forwarding from: %s has been stopped\n", fromChName),
			}
			m.outMsg <- msg
			change = true
		} else {
			fmt.Fprintf(&state.OutputStr, "Message forwarding to: %s doesnot exist\n", toChName)
		}
	}

	if change {
		m.writeToDB()
	}

	return nil
}
