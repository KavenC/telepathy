package telepathy

import (
	"context"
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

// ExtraCmdArgs defines the extra arguments passed to Command.Run callbacks
// All Command.Run callbacks must call ParseExtraCmdArgs to get these data
type ExtraCmdArgs struct {
	Ctx     context.Context
	Message *InboundMessage
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

// CommandParseExtraArgs parse extra command arguments for the Command.Run callbacks
func CommandParseExtraArgs(logger *logrus.Entry, extras ...interface{}) *ExtraCmdArgs {
	if len(extras) != 2 {
		logger.Panic("Insufficient arguments in command handler.")
	}

	ctx, ctxok := extras[0].(context.Context)
	message, msgok := extras[1].(*InboundMessage)
	if !ctxok || !msgok {
		logger.Panicf("Invalid argument type in command handler: %T, %T", extras[0], extras[1])
	}

	return &ExtraCmdArgs{
		Ctx:     ctx,
		Message: message,
	}
}

// CommandEnsureDM checks if command is from direct message
func CommandEnsureDM(cmd *cobra.Command, extraArgs *ExtraCmdArgs) bool {
	if !extraArgs.Message.IsDirectMessage {
		cmd.Print("This command can only be run with Direct Messages (Whispers).")
		return false
	}

	return true
}

// CommandGetPrefix returns the command prefix
func CommandGetPrefix() string {
	return commandPrefix
}

func isCmdMsg(text string) bool {
	return strings.HasPrefix(text, commandPrefix+" ")
}

func getCmdFromMsg(test string) []string {
	return regexp.MustCompile(" +").Split(test, -1)[1:]
}
