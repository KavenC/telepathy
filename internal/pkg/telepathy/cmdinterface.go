package telepathy

import (
	"errors"
	"regexp"
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

var commandPrefix = "#tele#"

var rootCmd = &cobra.Command{
	Use: commandPrefix,
	DisableFlagsInUseLine: true,
	Run: func(*cobra.Command, []string, ...interface{}) {
		// Do nothing
	},
}

func init() {
	rootCmd.Flags().BoolP("help", "h", false, "Show help for telepathy messenger commands")
	rootCmd.SetHelpTemplate(`== Telepathy messenger command interface ==
{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}
{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`)
	rootCmd.SetUsageTemplate(`* Usage:
  {{.CommandPath}} {{if .HasAvailableSubCommands}}[command]{{end}}{{if gt (len .Aliases) 0}}

* Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

* Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

* Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

* Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

* Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

* Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Send "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)
}

// RegisterCommand register a subcommand in telepathy command tree
func RegisterCommand(cmd *cobra.Command) error {
	logrus.WithField("command", cmd.Use).Info("Registering command")
	subcmds := rootCmd.Commands()
	for _, subcmd := range subcmds {
		if subcmd.Use == cmd.Use {
			return errors.New("Command already exists: " + cmd.Use)
		}
	}
	rootCmd.AddCommand(cmd)
	return nil
}

func isCmdMsg(text string) bool {
	return strings.HasPrefix(text, commandPrefix+" ")
}

func getCmdFromMsg(test string) []string {
	return regexp.MustCompile(" +").Split(test, -1)[1:]
}
