package twitch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func (s *twitchService) CommandInterface() *argo.Action {
	cmd := &argo.Action{
		Trigger:    s.ID(),
		ShortDescr: "Twitch Subscribe Service",
	}

	cmd.AddSubAction(argo.Action{
		Trigger:    "substream",
		ShortDescr: "Get notification when the user's stream changes",
		ArgNames:   []string{"user-name"},
		MinConsume: 1,
		Do:         s.subStream,
	})

	cmd.AddSubAction(argo.Action{
		Trigger:    "unsubstream",
		ShortDescr: "Unsubscribe to user's stream notifications",
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

func (s *twitchService) subStream(state *argo.State, extraArgs ...interface{}) error {
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
		ok, err := s.streamChangedAdd(User.ID, fromChannel)
		if err != nil {
			localLogger.Error(err.Error())
			fmt.Fprintf(&state.OutputStr, "Error when registering to Twitch webhook.")
		}
		if !ok {
			fmt.Fprintf(&state.OutputStr, "This channel has already subscribed to Twitch User: %s", User.DisplayName)
			return nil
		}
		fmt.Fprintf(&state.OutputStr, "Successfully subscribed to Twitch User: %s", User.DisplayName)
	case err := <-errChan:
		localLogger.Error(err.Error())
		fmt.Fprintf(&state.OutputStr, "Internal error.")
	case <-ctx.Done():
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}

	return nil
}

func (s *twitchService) unsubStream(state *argo.State, extraArgs ...interface{}) error {
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
	return nil
}

func (s *twitchService) queryUser(state *argo.State, extraArgs ...interface{}) error {
	errChan := make(chan error)
	respChan := make(chan *User)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	user := state.Args()[0]
	defer cancel()
	go s.api.fetchUser(ctx, user, respChan, errChan)
	select {
	case User := <-respChan:
		if User == nil {
			fmt.Fprintf(&state.OutputStr, "Twitch User not found: %s", user)
			return nil
		}
		fmt.Fprintf(&state.OutputStr, `== Twitch User ==
- Name: %s
- User ID: %s
- Login Name: %s
- Channel Description:
%s
- Channel View Count: %d
`, User.DisplayName, User.ID, User.Login, User.Description, User.ViewCount)
	case err := <-errChan:
		s.logger.WithField("cmd", "queryUser").Error(err.Error())
		fmt.Fprintf(&state.OutputStr, "Internal Error")
	case <-ctx.Done():
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}

	return nil
}

func (s *twitchService) queryStream(state *argo.State, extraArgs ...interface{}) error {
	errChan := make(chan error)
	respChan := make(chan Stream)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	user := state.Args()[0]
	go s.api.fetchStream(ctx, user, respChan, errChan)
	select {
	case stream := <-respChan:
		if !stream.Online {
			fmt.Fprintf(&state.OutputStr, `== Twitch Stream ==
- User: %s, stream offline or User not found.
`, user)
			return nil
		}
		fmt.Fprintf(&state.OutputStr, `== Twitch Stream ==
- Title: %s
- Streamer: %s
- Viewer Count: %d
- Game: %s
- Link: %s`, stream.Title, stream.UserName, stream.ViewerCount, stream.GameID, twitchURL+user)
	case err := <-errChan:
		s.logger.WithField("cmd", "queryStream").Error(err.Error())
		fmt.Fprintf(&state.OutputStr, "Internal Error")
	case <-ctx.Done():
		fmt.Fprintf(&state.OutputStr, "Request timeout, please try again later.")
	}
	return nil
}
