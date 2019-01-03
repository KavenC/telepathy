package plurkrss

import (
	"net/http"

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

	// Register Webhook for incoming plurk new post notifications
	telepathy.RegisterWebhook("plurk", webhook)
}

func webhook(response http.ResponseWriter, _ *http.Request) {
	// Parse new plurk post notification
	response.WriteHeader(200)
}

func subscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	msg := extraArgs.Message.FromChannel.MessengerID
	cid := extraArgs.Message.FromChannel.ChannelID
	user := args[0]

	omsg := &telepathy.OutboundMessage{
		TargetID: cid,
		Text:     "sub " + user,
	}

	msgr, err := extraArgs.Session.Msgr.Messenger(msg)
	if err != nil {
		return
	}
	msgr.Send(omsg)
}

func list(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	msg := extraArgs.Message.FromChannel.MessengerID
	cid := extraArgs.Message.FromChannel.ChannelID

	omsg := &telepathy.OutboundMessage{
		TargetID: cid,
		Text:     "list",
	}

	msgr, err := extraArgs.Session.Msgr.Messenger(msg)
	if err != nil {
		return
	}
	msgr.Send(omsg)
}

func unsubscribe(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	msg := extraArgs.Message.FromChannel.MessengerID
	cid := extraArgs.Message.FromChannel.ChannelID
	user := args[0]

	omsg := &telepathy.OutboundMessage{
		TargetID: cid,
		Text:     "unsub " + user,
	}

	msgr, err := extraArgs.Session.Msgr.Messenger(msg)
	if err != nil {
		return
	}
	msgr.Send(omsg)
}
