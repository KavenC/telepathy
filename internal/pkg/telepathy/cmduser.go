package telepathy

import (
	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

func init() {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Telepathy user management",
		Run: func(*cobra.Command, []string, ...interface{}) {
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

func newCmdHandle(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := CommandParseExtraArgs(
		logrus.WithField("command", args),
		extras...)

	isDM := CommandEnsureDM(cmd, extraArgs)

	if !isDM {
		return
	}
}
