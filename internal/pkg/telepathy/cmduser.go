package telepathy

import (
	"github.com/spf13/cobra"
)

func init() {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Telepathy user management",
		Run:   userCmdHandle,
	}
	RegisterCommand(userCmd)
}

func userCmdHandle(cmd *cobra.Command, args []string) {
	cmd.Print("Hi User.")
}
