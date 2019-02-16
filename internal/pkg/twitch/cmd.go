package twitch

import (
	"context"
	"time"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"

	"github.com/KavenC/cobra"
)

func (s *twitchService) CommandInterface() *cobra.Command {
	cmd := &cobra.Command{
		Use:   s.ID(),
		Short: "Twitch Sbuscribe Service",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "substream [user]",
		Example: "substream riorz",
		Short:   "Get notification when the user's stream changes",
		Args:    cobra.ExactArgs(1),
		Run:     s.subStream,
	})

	cmd.AddCommand(&cobra.Command{
		Use:     "unsubstream [user]",
		Example: "unsubstream riorz",
		Short:   "Unsubscribe to user stream notifications",
		Args:    cobra.ExactArgs(1),
		Run:     s.unsubStream,
	})

	cmd.AddCommand(&cobra.Command{
		Hidden:  true,
		Use:     "user [user]",
		Example: "user riorz",
		Short:   "Get user information",
		Args:    cobra.ExactArgs(1),
		Run:     s.queryUser,
	})

	cmd.AddCommand(&cobra.Command{
		Hidden:  true,
		Use:     "stream [user]",
		Example: "stream riorz",
		Short:   "Get user stream information",
		Args:    cobra.ExactArgs(1),
		Run:     s.queryStream,
	})

	return cmd
}

const reqTimeOut = 5 * time.Second

func (s *twitchService) subStream(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	streamUser := args[0]
	errChan := make(chan error)
	respChan := make(chan *User)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchUser(ctx, streamUser, respChan, errChan)

	localLogger := s.logger.WithField("cmd", "stream")

	extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
	if !ok {
		localLogger.Error("invalid extra args")
		cmd.Print("Internal error.")
		return
	}

	fromChannel := extArg.Message.FromChannel

	select {
	case user := <-respChan:
		if user == nil {
			cmd.Printf("Twitch user not found: %s", streamUser)
			return
		}
		ok, err := s.streamChangedAdd(user.ID, fromChannel)
		if err != nil {
			localLogger.Error(err.Error())
			cmd.Print("Error when registering to Twitch webhook.")
		}
		if !ok {
			cmd.Printf("This channel has already subscribed to Twitch user: %s", user.DisplayName)
			return
		}
		cmd.Printf("Successfully subscribed to Twitch user: %s", user.DisplayName)
	case err := <-errChan:
		localLogger.Error(err.Error())
		cmd.Print("Internal error.")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}

func (s *twitchService) unsubStream(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	streamUser := args[0]
	errChan := make(chan error)
	respChan := make(chan *User)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchUser(ctx, streamUser, respChan, errChan)

	localLogger := s.logger.WithField("cmd", "stream")

	extArg, ok := extraArgs[0].(telepathy.CmdExtraArgs)
	if !ok {
		localLogger.Error("invalid extra args")
		cmd.Print("Internal error.")
		return
	}

	fromChannel := extArg.Message.FromChannel

	select {
	case user := <-respChan:
		if user == nil {
			cmd.Printf("Twitch user not found: %s", streamUser)
			return
		}
		ok, err := s.streamChangedDel(user.ID, fromChannel)
		if err != nil {
			localLogger.Error(err.Error())
			cmd.Print("Internal Error.")
		}
		if !ok {
			cmd.Printf("This channel has not yet subscribed to Twitch user: %s", user.DisplayName)
			return
		}
		cmd.Printf("Successfully unsubscribed to Twitch user: %s", user.DisplayName)
	case err := <-errChan:
		localLogger.Error(err.Error())
		cmd.Print("Internal error.")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}

func (s *twitchService) queryUser(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	errChan := make(chan error)
	respChan := make(chan *User)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchUser(ctx, args[0], respChan, errChan)
	select {
	case user := <-respChan:
		if user == nil {
			cmd.Printf("Twitch user not found: %s", args[0])
			return
		}
		cmd.Printf(`== Twitch User ==
- Name: %s
- User ID: %s
- Login Name: %s
- Channel Description:
%s
- Channel View Count: %d
`, user.DisplayName, user.ID, user.Login, user.Description, user.ViewCount)
	case err := <-errChan:
		s.logger.WithField("cmd", "queryUser").Error(err.Error())
		cmd.Printf("Internal Error")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}

func (s *twitchService) queryStream(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	errChan := make(chan error)
	respChan := make(chan Stream)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchStream(ctx, args[0], respChan, errChan)
	select {
	case stream := <-respChan:
		if !stream.Online {
			cmd.Printf(`== Twitch Stream ==
- User: %s, stream offline or user not found.
`, args[0])
			return
		}
		cmd.Printf(`== Twitch Stream ==
- Title: %s
- Streamer: %s
- Viewer Count: %d
- Game: %s
- Link: %s`, stream.Title, stream.UserName, stream.ViewerCount, stream.GameID, twitchURL+args[0])
	case err := <-errChan:
		s.logger.WithField("cmd", "queryStream").Error(err.Error())
		cmd.Printf("Internal Error")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}
