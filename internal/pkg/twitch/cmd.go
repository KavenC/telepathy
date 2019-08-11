package twitch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// Command implements telepathy.PluginCommandHandler
func (s *Service) Command(done <-chan interface{}) *argo.Action {
	s.cmdDone = done
	cmd := &argo.Action{
		Trigger:    "twitch",
		ShortDescr: "Twitch Subscribe Service",
	}

	cmd.AddSubAction(argo.Action{
		Trigger:    "substream",
		ShortDescr: "Subscribe to stream change",
		ArgNames:   []string{"user-name"},
		MinConsume: 1,
		Do:         s.subStream,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "unsubstream",
		ShortDescr: "Unsubscribe to stream change",
		ArgNames:   []string{"user-name"},
		MinConsume: 1,
		Do:         s.unsubStream,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "user",
		ShortDescr: "Get user information",
		ArgNames:   []string{"user-name"},
		MinConsume: 1,
		Do:         s.queryUser,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "stream",
		ShortDescr: "Get user's stream information",
		ArgNames:   []string{"user-name"},
		MinConsume: 1,
		Do:         s.queryStream,
	})

	return cmd
}

const reqTimeOut = 5 * time.Second

func (s *Service) subStream(state *argo.State, extraArgs ...interface{}) error {
	userLogin := state.Args()[0]
	extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
	if !ok {
		return errors.New("invalid extra args")
	}

	ctx, cancel := context.WithTimeout(extArg.Ctx, reqTimeOut)
	defer cancel()
	userID, ok := <-s.api.userIDByLogin(ctx, userLogin)

	if !ok {
		return errors.New("subStream/userIDByLogin failed")
	}

	if userID == nil {
		fmt.Fprintf(&state.OutputStr, "Twitch user not found: %s", userLogin)
		return nil
	}

	// Check if already subscribed
	subtable := s.webhookSubs["streams"]
	channel := extArg.Message.FromChannel
	userIDExists, channelExists := subtable.lookUpOrAdd(*userID, channel)
	if channelExists {
		fmt.Fprintf(&state.OutputStr, "Already subscribed to user: %s", userLogin)
		return nil
	}

	streamChan := s.api.streamByLogin(ctx, userLogin)
	success := func() {
		fmt.Fprintf(&state.OutputStr, "Successfully subscribed to user: %s\n", userLogin)
		stream, ok := <-streamChan
		if !ok || stream == nil {
			return
		}
		var status string
		if stream.offline {
			status = "offline"
		} else {
			status = s.api.printStream(ctx, *stream, userLogin)
		}
		fmt.Fprintf(&state.OutputStr, "Current stream status:\n%s",
			status)
	}

	if userIDExists {
		success()
		return nil
	}

	// Do Websub flow
	subResult := s.api.subscribeStream(ctx, *userID)

	select {
	case _, subOk := <-subResult:
		if !subOk {
			subtable.remove(*userID, channel)
			return errors.New("subscribeStream failed")
		}
		success()
		return nil
	case <-ctx.Done():
		subtable.remove(*userID, channel)
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}
	return nil
}

func (s *Service) unsubStream(state *argo.State, extraArgs ...interface{}) error {
	/*
		streamUser := state.Args()[0]
		errChan := make(chan error)
		respChan := make(chan *User)
		ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
		defer cancel()
		go s.api.fetchUser(ctx, streamUser, respChan, errChan)

		localLogger := s.logger.WithField("cmd", "stream")

		extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
		if !ok {
			localLogger.Error("invalid extra args")
			return errors.New("invalid extra args")
		}

		fromChannel := extArg.Message.FromChannel

		select {
		case User := <-respChan:
			if User == nil {
				fmt.Fprintf(&state.OutputStr, "Twitch User not found: %s", streamUser)
				return nil
			}
			ok, err := s.streamChangedDel(User.ID, fromChannel)
			if err != nil {
				localLogger.Error(err.Error())
				state.OutputStr.WriteString("Internal Error.")
			}
			if !ok {
				fmt.Fprintf(&state.OutputStr, "This channel has not yet subscribed to Twitch User: %s", User.DisplayName)
				return nil
			}
			fmt.Fprintf(&state.OutputStr, "Successfully unsubscribed to Twitch User: %s", User.DisplayName)
		case err := <-errChan:
			localLogger.Error(err.Error())
			state.OutputStr.WriteString("Internal error.")
		case <-ctx.Done():
			fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
		}
	*/
	return nil
}

func (s *Service) queryUser(state *argo.State, extraArgs ...interface{}) error {
	extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
	if !ok {
		return errors.New("invalid extra args")
	}

	ctx, cancel := context.WithTimeout(extArg.Ctx, reqTimeOut)
	userLogin := state.Args()[0]
	defer cancel()
	respChan := s.api.userByLogin(ctx, userLogin)

	select {
	case user, ok := <-respChan:
		if !ok {
			return errors.New("getUserByName failed")
		}

		if len(user.ID) == 0 {
			fmt.Fprintf(&state.OutputStr, "Twitch user not found: %s", userLogin)
			return nil
		}

		fmt.Fprintf(&state.OutputStr, `== Twitch User ==
- Login Name: %s
- Display Name: %s
- Description:
%s
`, user.Login, user.DisplayName, user.Description)
	case <-ctx.Done():
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}

	return nil
}

func (s *Service) queryStream(state *argo.State, extraArgs ...interface{}) error {
	extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
	if !ok {
		return errors.New("invalid extra args")
	}

	ctx, cancel := context.WithTimeout(extArg.Ctx, reqTimeOut)
	userLogin := state.Args()[0]
	defer cancel()
	respChan := s.api.streamByLogin(ctx, userLogin)

	select {
	case stream := <-respChan:
		if stream == nil {
			fmt.Fprintf(&state.OutputStr, "User: %s not found", userLogin)
			return nil
		}
		if stream.offline {
			fmt.Fprintf(&state.OutputStr, `== Twitch Stream ==
- User: %s, stream offline
		`, userLogin)
			return nil
		}

		fmt.Fprintf(&state.OutputStr, "== Twitch Stream ==\n%s",
			s.api.printStream(ctx, *stream, userLogin))
	case <-ctx.Done():
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}

	return nil
}
