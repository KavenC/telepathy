package telepathy

import (
	"context"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
)

type cmdManager struct {
	session *Session
	rootCmd argo.Action
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

var regexCmdSplitter = regexp.MustCompile(" +")

func (e CmdExistsError) Error() string {
	return e.Cmd + " already exists."
}

func newCmdManager(s *Session) *cmdManager {
	return &cmdManager{
		session: s,
		rootCmd: argo.Action{Trigger: CommandPrefix},
		logger:  logrus.WithField("module", "cmdManager"),
	}
}

// RegisterCommand register a subcommand in telepathy command tree
func (m *cmdManager) RegisterCommand(cmd *argo.Action) error {
	err := m.rootCmd.AddSubAction(*cmd)
	if err != nil {
		_, ok := err.(argo.DuplicatedSubActionError)
		if ok {
			m.logger.Errorf("command: %s has already been registered", cmd.Trigger)
			return CmdExistsError{Cmd: cmd.Trigger}
		}
		m.logger.Errorf("RegisterCommand: %s", err.Error())
		return err
	}
	m.logger.Infof("registered command: %s", cmd.Trigger)
	return nil
}

func (m *cmdManager) handleCmdMsg(ctx context.Context, message *InboundMessage) {
	// Got a command message
	// Parse it with command interface
	args := regexCmdSplitter.Split(message.Text, -1)

	// Execute command
	extraArgs := CmdExtraArgs{
		Ctx:     ctx,
		Message: message,
	}

	state := &argo.State{}
	err := m.rootCmd.Parse(state, args, extraArgs)
	if err != nil {
		m.logger.Errorf("handleCmdMsg: %s", err.Error())
		replyMsg := &OutboundMessage{
			TargetID: message.FromChannel.ChannelID,
			Text:     "Internal Error",
		}
		msg, _ := m.session.Message.Messenger(message.FromChannel.MessengerID)
		msg.Send(replyMsg)
		return
	}

	// If there is stirng output, forward it back to messenger
	reply := state.OutputStr.String()
	if len(reply) > 0 {
		replyMsg := &OutboundMessage{
			TargetID: message.FromChannel.ChannelID,
			Text:     reply,
		}
		msg, _ := m.session.Message.Messenger(message.FromChannel.MessengerID)
		msg.Send(replyMsg)
	}
}

// CommandEnsureDM checks if command is from direct message
func CommandEnsureDM(state *argo.State, extraArgs CmdExtraArgs) bool {
	if !extraArgs.Message.IsDirectMessage {
		state.OutputStr.WriteString("This command can only be run with Direct Messages (Whispers).\n")
		return false
	}
	return true
}

func isCmdMsg(text string) bool {
	return strings.HasPrefix(text, CommandPrefix+" ")
}
