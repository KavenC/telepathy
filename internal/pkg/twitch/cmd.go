package twitch

import "github.com/KavenC/cobra"

func (s *twitchSubService) CommandInterface() *cobra.Command {
	cmd := &cobra.Command{
		Use:   serviceID,
		Short: "Twitch Sbuscribe Service",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Hidden:  true,
		Use:     "stream [user]",
		Example: "stream riorz",
		Short:   "Get notification when the user's stream changes",
		Args:    cobra.ExactArgs(1),
		Run:     s.stream,
	})

	return cmd
}

func (s *twitchSubService) stream(cmd *cobra.Command, args []string, extraArgs ...interface{}) {

}
