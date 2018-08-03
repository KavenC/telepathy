package telepathy

import (
	"github.com/spf13/cobra"
)

func init() {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Telepathy user management",
		Run: func(cmd *cobra.Command, args []string) {
			// Do nothing
		},
	}

	userCmd.AddCommand(&cobra.Command{
		Use:   "new",
		Short: "Create a new Telepathy user linked to current app. (DM only)",
		Run:   newCmdHandle,
	})
	RegisterCommand(userCmd)
}

func newCmdHandle(cmd *cobra.Command, args []string) {
	cmd.Print("Hi User.")
}
