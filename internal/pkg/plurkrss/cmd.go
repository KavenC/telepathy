package plurkrss

import (
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const funcKey = "plurk"

var logger = logrus.WithField("module", funcKey)

func (m *plurkSubManager) CommandInterface() *cobra.Command {
	// Construct command interface
	cmd := &cobra.Command{
		Use:   funcKey,
		Short: "Plurk RSS Subscription Service",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "sub [username]",
		Example: "subscribe Regular",
		Short:   "Subscribe a plurk user and forwards post to current channel",
		Args:    cobra.ExactArgs(1),
		Run:     m.subscribe,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all subscriptions",
		Run:   m.list,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unsub [username]",
		Short: "Unsubscribes a plurk user",
		Run:   m.unsubscribe,
	})

	return cmd
}

func (m *plurkSubManager) subscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		cmd.Print("Internal error. Command failed.")
		return
	}

	channel := extraArgs.Message.FromChannel
	user := args[0]

	if user == "" {
		cmd.Print("Invalid Plurk user")
	}

	if m.createSub(&user, &channel) {
		cmd.Printf("Subscribed to Plurk user: " + user)
		m.writeToDB()
	} else {
		cmd.Printf("This channel has already subscribed to Plurk user: " + user)
	}
}

func (m *plurkSubManager) list(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		cmd.Print("Internal error. Command failed.")
		return
	}

	channel := extraArgs.Message.FromChannel

	subs := m.subscriptions(&channel)
	cmd.Printf("This channel is subscribing Plurk user(s): %s", strings.Join(subs, ", "))
}

func (m *plurkSubManager) unsubscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs, ok := extras[0].(telepathy.CmdExtraArgs)
	if !ok {
		m.logger.Errorf("failed to parse extraArgs: %T", extras[0])
		cmd.Print("Internal error. Command failed.")
		return
	}

	channel := extraArgs.Message.FromChannel
	user := args[0]

	if user == "" {
		cmd.Print("Invalid Plurk user")
		return
	}

	if m.removeSub(&user, &channel) {
		cmd.Printf("Unsubscribed to Plurk user: " + user)
		m.writeToDB()
	} else {
		cmd.Printf("This channel has not subscribed to Plurk user: " + user)
	}
}
