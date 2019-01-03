package plurkrss

import (
	"strings"

	"github.com/KavenC/cobra"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

var logger = logrus.WithField("module", "plurkrss")

const funcKey = "plurk"

func init() {
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
		Run:     subscribe,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all subscriptions",
		Run:   list,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unsub [username]",
		Short: "Unsubscribes a plurk user",
		Run:   unsubscribe,
	})

	telepathy.RegisterCommand(cmd)
}

func subscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	channel := extraArgs.Message.FromChannel
	user := args[0]

	if user == "" {
		cmd.Print("Invalid Plurk user")
	}

	subManager := manager(extraArgs.Session)
	if subManager == nil {
		return
	}

	if subManager.createSub(&user, &channel) {
		cmd.Printf("Subscribed to Plurk user: " + user)
		subManager.writeToDB()
	} else {
		cmd.Printf("This channel has already subscribed to Plurk user: " + user)
	}
}

func list(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)
	channel := extraArgs.Message.FromChannel
	subManager := manager(extraArgs.Session)
	if subManager == nil {
		return
	}

	subs := subManager.subscriptions(&channel)
	cmd.Printf("This channel is subscribing Plurk user(s): %s", strings.Join(subs, ", "))
}

func unsubscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	channel := extraArgs.Message.FromChannel
	user := args[0]

	if user == "" {
		cmd.Print("Invalid Plurk user")
		return
	}

	subManager := manager(extraArgs.Session)
	if subManager.removeSub(&user, &channel) {
		cmd.Printf("Unsubscribed to Plurk user: " + user)
		subManager.writeToDB()
	} else {
		cmd.Printf("This channel has not subscribed to Plurk user: " + user)
	}
}
