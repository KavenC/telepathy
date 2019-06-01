package fwd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
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

const (
	redisExpireTime = 5 * time.Minute
)

// Session defines a structure for forwarding setup session
// This structure must be able to serialized to put in redis
type Session struct {
	First       telepathy.Channel
	FirstAlias  string
	Second      telepathy.Channel
	SecondAlias string
	Cmd         int
}

// Index defines a structure for setup channel for forwarding setup session
// This structure must be able to serialized to put in redis
type Index struct {
	SessionKey string
	Index      int
}

// allocate randstr in redis for fwd setup session
// return a struct of allocated strings and/or error
func allocateKeys(redis *redis.Client) interface{} {
	const (
		count = 3 // number of keys to be allocated
		retry = 3 // retry count if a key is already allocated
		len   = 5 // number of characters for a key
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
			ok, ret.err = redis.SetNX(key, "", redisExpireTime).Result()
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
			// Clean up allocated keys
			redis.Del(ret.keys[:i]...)
			break
		}
	}
	return ret
}

// Setup redis entries for the fwd setup session
func (m *forwardingManager) initSession(ctx context.Context, session Session) (string, string, error) {
	logger := logger.WithField("phase", "initSession")

	// Push redis request to allocate 3 keys
	// the keys are used for:
	// 1. UID for current session
	// 2. Identify 1st channel
	// 3. Identify 2nd channel
	redisRetChannel := make(chan interface{})
	m.session.Redis.PushRequest(&telepathy.RedisRequest{
		Action: allocateKeys,
		Return: redisRetChannel,
	})

	// Initialize Session, FirstChannel, SecondChannel structures
	sessionStr, err := json.Marshal(session)
	if err != nil {
		logger.WithField("item", "session").Error("JSON Marshal Failed: " + err.Error())
		return "", "", InternalError{Msg: err.Error()}
	}
	first := Index{Index: firstCh}
	second := Index{Index: secondCh}

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

	sessionID := redisRet.keys[0]
	firstChID := redisRet.keys[1]
	secondChID := redisRet.keys[2]

	first.SessionKey = sessionID
	firstStr, _ := json.Marshal(first)
	second.SessionKey = sessionID
	secondStr, _ := json.Marshal(second)

	// Push redis request for fill contents for the allocated keys
	// 1. sessionID -> sesson management structure
	// 2. firstChID -> {sessionID, firstChMark}
	// 3. secondChID -> {sessionID, secondChMark}
	redisSetRet := make(chan interface{})
	m.session.Redis.PushRequest(&telepathy.RedisRequest{
		Action: func(redis *redis.Client) interface{} {
			internalErr := InternalError{Msg: "Redis key lost"}
			ok, err := redis.SetXX(sessionID, sessionStr, redisExpireTime).Result()
			if err != nil {
				return err
			} else if !ok {
				return internalErr
			}
			ok, err = redis.SetXX(firstChID, firstStr, redisExpireTime).Result()
			if err != nil {
				return err
			} else if !ok {
				return internalErr
			}
			ok, err = redis.SetXX(secondChID, secondStr, redisExpireTime).Result()
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

	return firstChID, secondChID, nil
}

// Entry point to start a setup session
func (m *forwardingManager) setupFwd(state *argo.State, session Session, extraArgs telepathy.CmdExtraArgs) {
	args := state.Args()
	session.FirstAlias = args[0]
	session.SecondAlias = args[1]
	fmt.Fprintf(&state.OutputStr, "1st Channel: %s\n", args[0])
	fmt.Fprintf(&state.OutputStr, "2nd Channel: %s\n", args[1])

	// allocate keys in the redis
	// if allocated, prompt to user to start the process
	key1, key2, err := m.initSession(extraArgs.Ctx, session)

	if err != nil {
		state.OutputStr.WriteString(err.Error())
	} else {
		prefix := telepathy.CommandPrefix
		state.OutputStr.WriteString("\nPlease follow these steps:\n")
		state.OutputStr.WriteString("1. Make sure Telepathy is enabled in both channels\n")
		fmt.Fprintf(&state.OutputStr, "2. Send: %s %s set %s to the 1st channel\n", prefix, funcKey, key1)
		fmt.Fprintf(&state.OutputStr, "3. Send: %s %s set %s to the 2nd channel", prefix, funcKey, key2)
	}
}

// try create fwd between from and to, and outputting messages for the results
func (m *forwardingManager) createFwd(from, to, this telepathy.Channel, alias Alias) string {
	insertRet := <-m.table.insert(from,
		TableEntry{
			Channel: to,
			Alias:   alias,
		})
	var ret string

	if !insertRet.ok {
		return fmt.Sprintf("Forwarding from %s to %s already exists.", alias.SrcAlias, alias.DstAlias)
	}

	fromMsg := strings.Builder{}
	fromMsg.WriteString("Start forwarding messages to ")
	fromMsg.WriteString(insertRet.DstAlias)
	if alias.DstAlias != insertRet.DstAlias {
		fromMsg.WriteString(" (renamed from ")
		fromMsg.WriteString(alias.DstAlias)
		fromMsg.WriteString(")")
	}
	toMsg := strings.Builder{}
	toMsg.WriteString("Receiving forwarded messages from ")
	toMsg.WriteString(insertRet.SrcAlias)
	if alias.SrcAlias != insertRet.SrcAlias {
		toMsg.WriteString(" (renamed from ")
		toMsg.WriteString(alias.SrcAlias)
		toMsg.WriteString(")")
	}
	outMsg := &telepathy.OutboundMessage{}

	// Send notifications to related channels
	// if the channel is the key-setting channel, return the notification string rather than send it directly
	var msgrHandler telepathy.GlobalMessenger
	if from == this {
		ret = fromMsg.String()
		outMsg.Text = toMsg.String()
		outMsg.TargetID = to.ChannelID
		msgrHandler, _ = m.session.Message.Messenger(to.MessengerID)
	} else {
		ret = toMsg.String()
		outMsg.Text = fromMsg.String()
		outMsg.TargetID = from.ChannelID
		msgrHandler, _ = m.session.Message.Messenger(from.MessengerID)
	}

	msgrHandler.Send(outMsg)
	return ret
}

func (m *forwardingManager) setKeyProcess(key string, channel telepathy.Channel) func(*redis.Client) interface{} {
	// Construct the Redis request action function
	// String returned by Action function should be used as command reply
	return func(client *redis.Client) interface{} {
		logger := logger.WithField("phase", "setKey")
		errInvalidKey := "Invalid key, or key time out. Please restart setup process"

		// Get Index to find the Session to set channel info
		cmdstr, err := client.Get(key).Result()
		if err != nil {
			if err == redis.Nil {
				return errInvalidKey
			}
		}

		ind := Index{}
		err = json.Unmarshal([]byte(cmdstr), &ind)
		if err != nil {
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
		if err != nil {
			client.Del(ind.SessionKey)
			return errInvalidKey
		}

		errSameFwd := `Cannot create forwarding in the same channel
Setup process is terminated`
		errUnknown := "Internal Error. Please restart setup process"

		ret := strings.Builder{}
		// Set channel info to Session field
		switch ch := ind.Index; ch {
		case firstCh:
			if session.Second == channel {
				client.Del(ind.SessionKey)
				return errSameFwd
			}
			session.First = channel
			fmt.Fprintf(&ret, "Successfully set this channel as: %s", session.FirstAlias)
		case secondCh:
			if session.First == channel {
				client.Del(ind.SessionKey)
				return errSameFwd
			}
			session.Second = channel
			fmt.Fprintf(&ret, "Successfully set this channel as: %s", session.SecondAlias)
		default:
			logger.Errorf("Got invalid Index.Index: %v", ind)
			return errUnknown
		}

		if session.First.ChannelID == "" || session.Second.ChannelID == "" {
			// If the other channel is not set, prompt user to continue the process
			ret.WriteString("\nPlease set another channel with provided key")

			// Write Session back to redis
			sessionStr, _ := json.Marshal(session)
			err = client.SetXX(ind.SessionKey, string(sessionStr), redisExpireTime).Err()
			if err != nil {
				logger.Error("Store back Session error: " + err.Error())
				return errUnknown
			}
		} else {
			// If both channels are set, remove Session from redis and create forwarding
			client.Del(ind.SessionKey)

			// Twoway or oneway?
			preCreateLog := logger.WithFields(logrus.Fields{
				"first_channel":  session.First,
				"second_channel": session.Second,
			})
			switch cmd := session.Cmd; cmd {
			case oneWay:
				alias := Alias{
					SrcAlias: session.FirstAlias,
					DstAlias: session.SecondAlias,
				}
				preCreateLog.Info("Creating one-way forwarding")
				ret.WriteString("\n")
				ret.WriteString(m.createFwd(session.First, session.Second, channel, alias))
			case twoWay:
				alias := Alias{
					SrcAlias: session.FirstAlias,
					DstAlias: session.SecondAlias,
				}
				preCreateLog.Info("Creating two-way forwarding")
				ret.WriteString("\n")
				ret.WriteString(m.createFwd(session.First, session.Second, channel, alias))
				ret.WriteString("\n")
				alias = Alias{
					SrcAlias: session.SecondAlias,
					DstAlias: session.FirstAlias,
				}
				ret.WriteString(m.createFwd(session.Second, session.First, channel, alias))
			default:
				preCreateLog.Errorf("got invalid Cmd in Session: %v", session)
				return errUnknown
			}
			m.writeToDB()
		}
		return ret.String()
	}
}
