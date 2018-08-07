package telepathy

import (
	"encoding/json"
	"time"

	"github.com/KavenC/cobra"
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
)

const (
	chCmdTwoWayFwd = 0
	chCmdOneWayFwd = 1
)

const (
	chCmdFirstCh  = 0
	chCmdSecondCh = 1
)

const chRedisType = "channel"

// ChCmdSession contains channel command session info
type ChCmdSession struct {
	EntryType  string
	FirstType  string
	FirstID    string
	SecondType string
	SecondID   string
	Cmd        int
}

// ChCmdInd contains channel info for channel commands
type ChCmdInd struct {
	EntryType  string
	SessionKey string
	ChannelInd int
}

func init() {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Telepathy channel management",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "2way",
		Short: "Create two-way channel forwarding (DM only)",
		Run:   cmdChannelTwoWay,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "1way",
		Short: "Create one-way channel forwarding (DM only)",
		Run:   cmdChannelOneWay,
	})

	cmd.AddCommand(&cobra.Command{
		Use:     "set",
		Example: "set [key]",
		Short:   "Used for identify channels various channel features.",
		Args:    cobra.ExactArgs(1),
		Run:     cmdChannelSet,
	})

	RegisterCommand(cmd)
}

func findAndSetKey(value string, expire time.Duration) (string, error) {
	var key string
	retry := 3
	redis := getRedis()
	defer putRedis()
	for ; retry > 0; retry-- {
		key = getRandStr(8)
		ok, err := redis.SetNX(key, value, expire).Result()
		if err != nil {
			return key, err
		}
		if ok {
			break
		}
	}

	if retry == 0 {
		return "", nil
	}

	return key, nil
}

func initChCmdSession(session *ChCmdSession) (string, string, error) {
	expire := time.Minute
	var err error
	jsonStr, err := json.Marshal(session)
	if err != nil {
		logrus.Panic("JSON Marshal Failed: " + err.Error())
	}
	mainKey, err := findAndSetKey(string(jsonStr), expire)
	if err != nil {
		return "", "", err
	}

	firstChInd := ChCmdInd{
		EntryType:  chRedisType,
		SessionKey: mainKey,
		ChannelInd: chCmdFirstCh,
	}
	jsonStr, err = json.Marshal(firstChInd)
	if err != nil {
		logrus.Panic("JSON Marshal Failed: " + err.Error())
	}
	firstKey, err := findAndSetKey(string(jsonStr), expire)
	if err != nil {
		getRedis().Del(mainKey)
		putRedis()
		return "", "", err
	}

	secondChInd := ChCmdInd{
		EntryType:  chRedisType,
		SessionKey: mainKey,
		ChannelInd: chCmdSecondCh,
	}
	jsonStr, err = json.Marshal(secondChInd)
	if err != nil {
		logrus.Panic("JSON Marshal Failed: " + err.Error())
	}
	secondKey, err := findAndSetKey(string(jsonStr), expire)
	if err != nil {
		getRedis().Del(mainKey, firstKey)
		putRedis()
		return "", "", err
	}

	return firstKey, secondKey, nil
}

func cmdChannelSetupFwd(cmd *cobra.Command, session *ChCmdSession) {
	key1, key2, err := initChCmdSession(session)
	prefix := CommandGetPrefix()

	if err != nil {
		cmd.Print(err)
	} else {
		cmd.Print(`Setup two-way channel forwarding in 3 steps:
1. Make sure Teruhashi is in both channels.
2. Send "` + prefix + " channel set " + key1 + `" to the first channel. (Withouth ").
3. Send "` + prefix + " channel set " + key2 + `" to the second channel. (Withouth ").`)
	}
}

func cmdChannelTwoWay(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := CommandParseExtraArgs(
		logrus.WithField("command", args),
		extras...)

	isDM := CommandEnsureDM(cmd, extraArgs)

	if !isDM {
		return
	}

	cmdSession := ChCmdSession{
		EntryType: chRedisType,
		Cmd:       chCmdTwoWayFwd,
	}

	cmdChannelSetupFwd(cmd, &cmdSession)
}

func cmdChannelOneWay(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := CommandParseExtraArgs(
		logrus.WithField("command", args),
		extras...)

	isDM := CommandEnsureDM(cmd, extraArgs)

	if !isDM {
		return
	}

	cmdSession := ChCmdSession{
		EntryType: chRedisType,
		Cmd:       chCmdOneWayFwd,
	}

	cmdChannelSetupFwd(cmd, &cmdSession)
}

func cmdChannelSet(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := CommandParseExtraArgs(
		logrus.WithField("command", args),
		extras...)

	msg := extraArgs.Message.Messenger.name()
	channelID := extraArgs.Message.SourceID
	key := args[0]

	client := getRedis()
	defer putRedis()

	cmdstr, err := client.Get(key).Result()
	if err != nil {
		if err == redis.Nil {
			cmd.Print("Invalid key, or key time out. Please restart setup process.")
			return
		} else {
			logrus.Panic(err)
		}
	}

	chind := ChCmdInd{}
	err = json.Unmarshal([]byte(cmdstr), &chind)
	if err != nil || chind.EntryType != chRedisType {
		cmd.Print("Invalid key.")
		return
	}
	client.Del(key)

	chsession := ChCmdSession{}
	cmdstr, err = client.Get(chind.SessionKey).Result()
	if err != nil {
		if err == redis.Nil {
			cmd.Print("Invalid key, or key time out. Please restart setup process.")
			return
		} else {
			logrus.Panic(err)
		}
	}

	err = json.Unmarshal([]byte(cmdstr), &chsession)
	if err != nil || chsession.EntryType != chRedisType {
		cmd.Print("Internal Error. Please restart setup process.")
		return
	}

	switch ch := chind.ChannelInd; ch {
	case chCmdFirstCh:
		chsession.FirstID = channelID
		chsession.FirstType = msg
		cmd.Print("Set as First Channel successfully.")
	case chCmdSecondCh:
		chsession.SecondID = channelID
		chsession.SecondType = msg
		cmd.Print("Set as Second Channel successfully.")
	default:
		cmd.Print("Internal Error. Please restart setup process.")
		logrus.Error("Got invalid ChannelInd in ChCmdInd.")
		return
	}

	cmd.Print("\n")

	if chsession.FirstID == "" || chsession.SecondID == "" {
		cmd.Print("Please set another channel with provided key.")
		jsonStr, err := json.Marshal(chsession)
		if err != nil {
			logrus.Panic("JSON Marshal Failed: " + err.Error())
		}
		err = client.SetXX(chind.SessionKey, string(jsonStr), time.Minute).Err()
		if err != nil {
			cmd.Print("Internal Error. Please restart setup process.")
			logrus.Error(err)
		}
	} else {
		cmd.Print("Cmd End.\n")
		client.Del(chind.SessionKey)

		switch chCmd := chsession.Cmd; chCmd {
		case chCmdOneWayFwd:
			cmd.Print("One way fowarding.\n")
			cmd.Printf("%v-%v -> %v-%v",
				chsession.FirstType, chsession.FirstID,
				chsession.SecondType, chsession.SecondID)
		case chCmdTwoWayFwd:
			cmd.Print("Two way fowarding.\n")
			cmd.Printf("%v-%v <-> %v-%v",
				chsession.FirstType, chsession.FirstID,
				chsession.SecondType, chsession.SecondID)
		default:
			cmd.Print("Internal Error. Please restart setup process.")
			logrus.Error("Got invalid Cmd in ChCmdSession.")
			return
		}
	}
}
