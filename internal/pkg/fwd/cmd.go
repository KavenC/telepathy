package fwd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KavenC/cobra"
	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	twoWay int = iota
	oneWay
)

const (
	firstCh int = iota
	secondCh
)

var logger = logrus.WithField("module", "fwd")

const funcKey = "fwd"

// Session defines a structure for forwarding setup session
type Session struct {
	EntryType  string
	FirstType  string
	FirstID    string
	SecondType string
	SecondID   string
	Cmd        int
}

// Index defines a structure for setup channel for forwarding setup session
type Index struct {
	EntryType  string
	SessionKey string
	Index      int
}

// PublicError is the type of error that is intended to feedback to user
// The error message will be returned to user
type PublicError struct {
	Msg string
}

// InternalError is the type of error that should not be disclosed to user
type InternalError struct {
	Msg string
}

// TerminatedError indicates system has be interanlly terminated
type TerminatedError struct {
	Msg string
}

func (e PublicError) Error() string {
	return e.Msg
}

func (e InternalError) Error() string {
	return e.Msg
}

func (e TerminatedError) Error() string {
	return "Terminated: " + e.Msg
}

func init() {
	cmd := &cobra.Command{
		Use:   "fwd",
		Short: "Cross-app Message Forwarding",
		Run: func(*cobra.Command, []string, ...interface{}) {
			// Do nothing
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "2way",
		Short: "Create two-way channel forwarding (DM only)",
		Run:   createTwoWay,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "1way",
		Short: "Create one-way channel forwarding (DM only)",
		Run:   createOneWay,
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "info",
		Short: "Show message forwarding info (form/to).",
		Run:   info,
	})

	cmd.AddCommand(&cobra.Command{
		Use:     "set",
		Example: "set [key]",
		Short:   "Used for identify channels various channel features.",
		Args:    cobra.ExactArgs(1),
		Run:     set,
	})

	telepathy.RegisterCommand(cmd)
}

func allocate3Keys(redis *redis.Client) interface{} {
	// allocate 3 keys in redis for a fwd session
	const (
		count  = 3
		retry  = 3
		len    = 8
		expire = time.Minute
	)

	ret := struct {
		keys [count]string
		err  error
	}{}

	for i := 0; i < count; i++ {
		r := retry
		for ; r > 0; r-- {
			key := telepathy.RandStr(len)
			var ok bool
			ok, ret.err = redis.SetNX(key, "", expire).Result()
			if ret.err != nil {
				return ret
			}
			if ok {
				ret.keys[i] = key
				break
			}
		}

		if r == 0 {
			ret.err = PublicError{
				Msg: "System busy, please try again later",
			}
			// Clean up
			redis.Del(ret.keys[:i]...)
			break
		}
	}

	return ret
}

func initSession(ctx context.Context, session *Session, t *telepathy.Session) (string, string, error) {
	logger := logger.WithField("phase", "initSession")
	// Allocate 3 keys in redis
	redisRetChannel := make(chan interface{})
	t.Redis.PushRequest(&telepathy.RedisRequest{
		Action: allocate3Keys,
		Return: redisRetChannel,
	})

	// Initialize Session, FirstChannel, SecondChannel structures
	var err error
	sesJSON, err := json.Marshal(session)
	if err != nil {
		logger.WithField("item", "session").Error("JSON Marshal Failed: " + err.Error())
		return "", "", InternalError{Msg: err.Error()}
	}
	first := Index{
		EntryType: funcKey,
		Index:     firstCh,
	}
	second := Index{
		EntryType: funcKey,
		Index:     secondCh,
	}

	// Wait here until Redis keys has been allocated
	var tempRet interface{}
	select {
	case <-ctx.Done():
		logger.Warn("Context terminated")
		return "", "", TerminatedError{}
	case tempRet = <-redisRetChannel:
		// Fall through
	}

	redisRet, _ := tempRet.(struct {
		keys [3]string
		err  error
	})

	if redisRet.err != nil {
		logger.Error("Failed to allocate keys: " + redisRet.err.Error())
		return "", "", redisRet.err
	}

	// Push redis request for session keys
	first.SessionKey = redisRet.keys[0]
	firstJSON, err := json.Marshal(first)
	if err != nil {
		logger.WithField("item", "first").Error("JSON Marshal Failed: " + err.Error())
		return "", "", InternalError{Msg: err.Error()}
	}
	second.SessionKey = redisRet.keys[0]
	secondJSON, err := json.Marshal(second)
	if err != nil {
		logger.WithField("item", "second").Error("JSON Marshal Failed: " + err.Error())
		return "", "", InternalError{Msg: err.Error()}
	}
	redisSetRet := make(chan interface{})
	t.Redis.PushRequest(&telepathy.RedisRequest{
		Action: func(redis *redis.Client) interface{} {
			expire := time.Minute
			internalErr := InternalError{Msg: "Redis key lost"}
			ok, err := redis.SetXX(redisRet.keys[0], sesJSON, expire).Result()
			if err != nil {
				return err
			} else if !ok {
				return internalErr
			}
			ok, err = redis.SetXX(redisRet.keys[1], firstJSON, expire).Result()
			if err != nil {
				return err
			} else if !ok {
				return internalErr
			}
			ok, err = redis.SetXX(redisRet.keys[2], secondJSON, expire).Result()
			if err != nil {
				return err
			} else if !ok {
				return internalErr
			}
			return nil
		},
		Return: redisSetRet,
	})

	select {
	case <-ctx.Done():
		logger.Warn("Context terminated")
		return "", "", TerminatedError{}
	case setRet := <-redisSetRet:
		err, _ := setRet.(error)
		if err != nil {
			logger.Error("Redis set key failed: " + err.Error())
			return "", "", err
		}
	}

	return redisRet.keys[1], redisRet.keys[2], nil
}

func setupFwd(cmd *cobra.Command, session *Session, extraArgs *telepathy.CmdExtraArgs) {
	key1, key2, err := initSession(extraArgs.Ctx, session, extraArgs.Session)
	prefix := telepathy.CommandPrefix

	if err != nil {
		cmd.Print(err)
	} else {
		cmd.Print(`
1. Make sure Teruhashi is in both channels.
2. Send "` + prefix + " fwd set " + key1 + `" to the first channel. (Without: ").
3. Send "` + prefix + " fwd set " + key2 + `" to the second channel. (Without: ").`)
	}
}

func createTwoWay(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	if !telepathy.CommandEnsureDM(cmd, extraArgs) {
		return
	}

	session := Session{
		EntryType: funcKey,
		Cmd:       twoWay,
	}

	cmd.Print("Setup two-way channel forwarding in following steps:")
	setupFwd(cmd, &session, extraArgs)
}

func createOneWay(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	if !telepathy.CommandEnsureDM(cmd, extraArgs) {
		return
	}

	session := Session{
		EntryType: funcKey,
		Cmd:       oneWay,
	}

	cmd.Print("Setup one-way channel forwarding in following steps:")
	setupFwd(cmd, &session, extraArgs)
}

func info(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	manager := manager(extraArgs.Session)
	toChList := manager.forwardingTo(extraArgs.Message.FromChannel)
	if toChList != nil {
		cmd.Print("= Messages are forwarding to:")
		for toCh := range toChList {
			cmd.Printf("\n%s", toCh.Name())
		}
	}

	fromChList := manager.forwardingFrom(extraArgs.Message.FromChannel)
	if fromChList != nil {
		cmd.Print("\n\n= Receiving forwarded messages from:")
		for fromCh := range fromChList {
			cmd.Printf("\n%s", fromCh.Name())
		}
	}

	if toChList == nil && fromChList == nil {
		cmd.Print("This channel is not in any forwarding pairs.")
	}
}

func createFwd(from, to, this *telepathy.Channel, s *telepathy.Session) string {
	fwd := manager(s)
	ok := fwd.createForwarding(*from, *to)
	var ret string

	if !ok {
		return fmt.Sprintf("Forwarding from %s to %s already exists.", from.Name(), to.Name())
	}
	fromMsg := "Start forwarding messages to " + to.Name()
	toMsg := "Receiving forwarded messages from " + from.Name()
	outMsg := &telepathy.OutboundMessage{}
	var msgrHandler telepathy.GlobalMessenger
	if *from == *this {
		ret = fromMsg
		outMsg.Text = toMsg
		outMsg.TargetID = to.ChannelID
		msgrHandler, _ = s.Msgr.Messenger(to.MessengerID)
	} else {
		ret = toMsg
		outMsg.Text = fromMsg
		outMsg.TargetID = from.ChannelID
		msgrHandler, _ = s.Msgr.Messenger(from.MessengerID)
	}
	msgrHandler.Send(outMsg)

	return ret
}

func setKeyProcess(key, cid, msg string, s *telepathy.Session) func(*redis.Client) interface{} {
	// Construct the Redis request action function
	// String returned by Action function should be used as command reply
	return func(client *redis.Client) interface{} {
		logger := logger.WithField("phase", "setKey")
		errInvalidKey := "Invalid key, or key time out. Please restart setup process."

		// Get Index to find the Session to set channel info
		cmdstr, err := client.Get(key).Result()
		if err != nil {
			if err == redis.Nil {
				return errInvalidKey
			}
		}

		ind := Index{}
		err = json.Unmarshal([]byte(cmdstr), &ind)
		if err != nil || ind.EntryType != funcKey {
			return errInvalidKey
		}
		client.Del(key)

		// Get Session
		session := Session{}
		cmdstr, err = client.Get(ind.SessionKey).Result()
		if err != nil {
			if err == redis.Nil {
				return errInvalidKey
			}
			logger.Error("Session key get failed: " + err.Error())
		}

		err = json.Unmarshal([]byte(cmdstr), &session)
		if err != nil || session.EntryType != funcKey {
			return errInvalidKey
		}

		errSameFwd := "Cannot create forwarding in the same channel.\n" +
			"Please restart setup process."
		errUnknown := "Internal Error. Please restart setup process."
		ret := ""
		// Set channel info to Session field
		switch ch := ind.Index; ch {
		case firstCh:
			if session.SecondID == cid && session.SecondType == msg {
				client.Del(ind.SessionKey)
				return errSameFwd
			}
			session.FirstID = cid
			session.FirstType = msg
			ret += "Set as First Channel successfully."
		case secondCh:
			if session.FirstID == cid && session.FirstType == msg {
				client.Del(ind.SessionKey)
				return errSameFwd
			}
			session.SecondID = cid
			session.SecondType = msg
			ret += "Set as Second Channel successfully."
		default:
			logger.Errorf("Got invalid Index.Index: %v", ind)
			return errUnknown
		}

		ret += "\n"

		if session.FirstID == "" || session.SecondID == "" {
			// If the other channel is not set, prompt user to continue the process
			ret += "Please set another channel with provided key."
			jsonStr, err := json.Marshal(session)
			if err != nil {
				logger.Error("JSON Marshal Failed: " + err.Error())
				return errUnknown
			}
			err = client.SetXX(ind.SessionKey, string(jsonStr), time.Minute).Err()
			if err != nil {
				logger.Error("Store back Session error: " + err.Error())
				return errUnknown
			}
		} else {
			// If both channels are set, remove Session from redis and create forwarding
			client.Del(ind.SessionKey)
			fch := &telepathy.Channel{
				MessengerID: session.FirstType,
				ChannelID:   session.FirstID,
			}
			sch := &telepathy.Channel{
				MessengerID: session.SecondType,
				ChannelID:   session.SecondID,
			}
			tch := &telepathy.Channel{
				MessengerID: msg,
				ChannelID:   cid,
			}

			// Twoway or oneway?
			switch cmd := session.Cmd; cmd {
			case oneWay:
				logger.WithFields(logrus.Fields{
					"first_type":     session.FirstType,
					"first_channel":  session.FirstID,
					"second_type":    session.SecondType,
					"second_channel": session.SecondID,
				}).Info("Creating one-way forwarding")
				ret += createFwd(fch, sch, tch, s)
			case twoWay:
				logger.WithFields(logrus.Fields{
					"first_type":     session.FirstType,
					"first_channel":  session.FirstID,
					"second_type":    session.SecondType,
					"second_channel": session.SecondID,
				}).Info("Creating two-way forwarding")
				ret += createFwd(fch, sch, tch, s)
				ret += "\n" + createFwd(sch, fch, tch, s)
			default:
				logger.Errorf("Got invalid Cmd in Session: %v", session)
				return errUnknown
			}
			manager(s).writeToDB()
		}
		return ret
	}
}

func set(cmd *cobra.Command, args []string, extras ...interface{}) {
	extraArgs := telepathy.NewCmdExtraArgs(extras...)

	msg := extraArgs.Message.FromChannel.MessengerID
	cid := extraArgs.Message.FromChannel.ChannelID
	key := args[0]

	// Set key in redis
	redisRet := make(chan interface{})
	extraArgs.Session.Redis.PushRequest(&telepathy.RedisRequest{
		Action: setKeyProcess(key, cid, msg, extraArgs.Session),
		Return: redisRet,
	})

	// Wait for reply
	select {
	case <-extraArgs.Ctx.Done():
		logger.Warn("Terminated")
	case reply := <-redisRet:
		replyStr, _ := reply.(string)
		if replyStr != "" {
			cmd.Print(replyStr)
		}
	}
}
