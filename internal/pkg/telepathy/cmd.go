package telepathy

import (
	"context"
	"regexp"
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
)

type cmdManager struct {
	session *Session
	rootCmd *cobra.Command
	logger  *logrus.Entry
}

// CmdExtraArgs defines the extra arguments passed to Command.Run callbacks
// All Command.Run callbacks must call ParseExtraCmdArgs to get these data
type CmdExtraArgs struct {
	Ctx     context.Context
	Message *InboundMessage
}

// CmdExistsError indicates registering an already registered command
type CmdExistsError struct {
	Cmd string
}

// CommandPrefix is the trigger word for the Telepathy command message
const CommandPrefix = "teru"

func rootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:                   CommandPrefix,
		DisableFlagsInUseLine: true,
		Args:                  cobra.MinimumNArgs(1),
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}
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
	return rootCmd
}

func (e CmdExistsError) Error() string {
	return e.Cmd + " already exists."
}

func newCmdManager(s *Session) *cmdManager {
	return &cmdManager{
		session: s,
		rootCmd: rootCmd(),
		logger:  logrus.WithField("module", "cmdManager"),
	}
}

// RegisterCommand register a subcommand in telepathy command tree
func (m *cmdManager) RegisterCommand(cmd *cobra.Command) error {
	subcmds := m.rootCmd.Commands()
	for _, subcmd := range subcmds {
		if subcmd.Use == cmd.Use {
			m.logger.Errorf("command: %s has already been registered", cmd.Use)
			return CmdExistsError{Cmd: cmd.Use}
		}
	}
	m.rootCmd.AddCommand(cmd)
	m.logger.Infof("registered command: %s", cmd.Use)
	return nil
}

func (m *cmdManager) handleCmdMsg(ctx context.Context, message *InboundMessage) {
	// Got a command message
	// Parse it with command interface
	args := regexp.MustCompile(" +").Split(message.Text, -1)[1:]

	cmd := *m.rootCmd // make a copy to be goroutine safe

	cmd.SetArgs(args)
	var buffer strings.Builder
	cmd.SetOutput(&buffer)

	// Execute command
	extraArgs := CmdExtraArgs{
		Ctx:     ctx,
		Message: message,
	}
	cmd.Execute(extraArgs)

	// If there is stirng output, forward it back to messenger
	if buffer.Len() > 0 {
		replyMsg := &OutboundMessage{
			TargetID: message.FromChannel.ChannelID,
			Text:     buffer.String(),
		}
		msg, _ := m.session.Message.Messenger(message.FromChannel.MessengerID)
		msg.Send(replyMsg)
	}
}

// CommandEnsureDM checks if command is from direct message
func CommandEnsureDM(cmd *cobra.Command, extraArgs CmdExtraArgs) bool {
	if !extraArgs.Message.IsDirectMessage {
		cmd.Print("This command can only be run with Direct Messages (Whispers).")
		return false
	}
	return true
}

func isCmdMsg(text string) bool {
	return strings.HasPrefix(text, CommandPrefix+" ")
}
