package twitch

import (
	"context"
	"time"

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
		Use:     "sub-stream [user]",
		Example: "sub-stream riorz",
		Short:   "Get notification when the user's stream changes",
		Args:    cobra.ExactArgs(1),
		Run:     s.subStream,
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

}

func (s *twitchService) queryUser(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	errChan := make(chan error, 1)
	respChan := make(chan User, 1)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchUser(ctx, args[0], respChan, errChan)
	select {
	case user, _ := <-respChan:
		cmd.Printf(`== Twitch User ==
- Name: %s
- User ID: %s
- Login Name: %s
- Channel Description:
%s
- Channel View Count: %d
`, user.DisplayName, user.ID, user.Login, user.Description, user.ViewCount)
	case err, _ := <-errChan:
		s.logger.WithField("cmd", "queryUser").Error(err.Error())
		cmd.Printf("Internal Error")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}

func (s *twitchService) queryStream(cmd *cobra.Command, args []string, extraArgs ...interface{}) {
	errChan := make(chan error, 1)
	respChan := make(chan Stream, 1)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchStream(ctx, args[0], respChan, errChan)
	select {
	case stream, _ := <-respChan:
		if !stream.Online {
			cmd.Printf(`== Twitch Stream ==
- User: %s, stream offline or user not found.
`, args[0])
			return
		}
		cmd.Printf(`== Twitch Stream ==
- User Name: %s
- Stream Title: %s
- Stream ID: %s
- Type: %s
- Stream Viewer Count: %d
`, stream.UserName, stream.Title, stream.ID, stream.Type, stream.ViewerCount)
	case err, _ := <-errChan:
		s.logger.WithField("cmd", "queryStream").Error(err.Error())
		cmd.Printf("Internal Error")
	case <-ctx.Done():
		cmd.Printf("Request timeout, please try again later.")
	}
}
