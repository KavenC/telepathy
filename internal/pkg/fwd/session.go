package fwd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/randstr"
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
	keyExpireTime = 5 * time.Minute
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

type publicError struct {
	msg string
}

type internalError struct {
	msg string
}

type terminatedError struct {
}

func (e publicError) Error() string {
	return e.msg
}

func (e internalError) Error() string {
	return e.msg
}

func (e terminatedError) Error() string {
	return "terminated"
}

// allocate randstr in cache for fwd setup session
// return allocated keys. If failed to allocate, return nil
func (m *Service) allocateKeys() []string {
	logger := m.logger.WithField("phase", "allocateKeys")
	const (
		retry = 3 // retry count if a key is already allocated
		len   = 5 // number of characters for a key
	)

	ret := make([]string, 0, 3)

	for i := 0; i < 3; i++ {
		r := retry
		for ; r > 0; r-- {
			key := randstr.Generate(len)
			err := m.sessionKeys.Add(key, "", cache.DefaultExpiration)
			logger.Warnf("allocating key: %s", key)
			if err != nil {
				// key exists, retry
				continue
			}
			ret = append(ret, key)
			break
		}

		if r == 0 {
			logger.Warn("cache busy, failed to allocate")
			// Clean up allocated keys
			for _, key := range ret {
				m.sessionKeys.Delete(key)
			}
			return nil
		}
	}

	return ret
}

// Setup redis entries for the fwd setup session
func (m *Service) initSession(ctx context.Context, session Session) (string, string, error) {
	// the keys are used for:
	// 1. UID for current session
	// 2. Identify 1st channel
	// 3. Identify 2nd channel
	keys := m.allocateKeys()
	if keys == nil {
		return "", "", internalError{msg: "allocate key failed"}
	}

	// Initialize Session, FirstChannel, SecondChannel structures
	first := Index{Index: firstCh}
	second := Index{Index: secondCh}

	sessionID := keys[0]
	firstChID := keys[1]
	secondChID := keys[2]

	first.SessionKey = sessionID
	second.SessionKey = sessionID

	// Push redis request for fill contents for the allocated keys
	// 1. sessionID -> sesson management structure
	// 2. firstChID -> {sessionID, firstChMark}
	// 3. secondChID -> {sessionID, secondChMark}
	err := m.sessionKeys.Replace(sessionID, session, cache.DefaultExpiration)
	if err != nil {
		return "", "", internalError{msg: fmt.Sprintf("set key failed: %s", err.Error())}
	}
	err = m.sessionKeys.Replace(firstChID, first, cache.DefaultExpiration)
	if err != nil {
		return "", "", internalError{msg: fmt.Sprintf("set key failed: %s", err.Error())}
	}
	err = m.sessionKeys.Replace(secondChID, second, cache.DefaultExpiration)
	if err != nil {
		return "", "", internalError{msg: fmt.Sprintf("set key failed: %s", err.Error())}
	}

	return firstChID, secondChID, nil
}

// Entry point to start a setup session
func (m *Service) setupFwd(state *argo.State, session Session, extraArgs telepathy.CmdExtraArgs) error {
	args := state.Args()
	session.FirstAlias = args[0]
	session.SecondAlias = args[1]
	fmt.Fprintf(&state.OutputStr, "1st Channel: %s\n", args[0])
	fmt.Fprintf(&state.OutputStr, "2nd Channel: %s\n", args[1])

	// allocate keys in the redis
	// if allocated, prompt to user to start the process
	key1, key2, err := m.initSession(extraArgs.Ctx, session)
	if err != nil {
		return err
	}

	prefix := telepathy.CommandPrefix()
	state.OutputStr.WriteString("\nPlease follow these steps:\n")
	state.OutputStr.WriteString("1. Make sure Telepathy is enabled in both channels\n")
	fmt.Fprintf(&state.OutputStr, "2. Send: %s %s set %s to the 1st channel\n", prefix, funcKey, key1)
	fmt.Fprintf(&state.OutputStr, "3. Send: %s %s set %s to the 2nd channel", prefix, funcKey, key2)
	return nil
}

// try create fwd between from and to, and outputting messages for the results
func (m *Service) createFwd(from, to, this telepathy.Channel, alias Alias) string {
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
	outMsg := telepathy.OutboundMessage{}

	// Send notifications to related channels
	// if the channel is the key-setting channel, return the notification string rather than send it directly
	if from == this {
		ret = fromMsg.String()
		outMsg.Text = toMsg.String()
		outMsg.ToChannel = to
	} else {
		ret = toMsg.String()
		outMsg.Text = fromMsg.String()
		outMsg.ToChannel = from
	}

	m.outMsg <- outMsg
	return ret
}

func (m *Service) setKeyProcess(key string, channel telepathy.Channel) (string, error) {
	// Construct the Redis request action function
	// String returned by Action function should be used as command reply
	errInvalidKey := "Invalid key, or key time out. Please restart setup process"

	value, ok := m.sessionKeys.Get(key)
	if !ok {
		return errInvalidKey, nil
	}

	index, ok := value.(Index)
	if !ok {
		return errInvalidKey, nil
	}
	m.sessionKeys.Delete(key)

	// Get session
	value, ok = m.sessionKeys.Get(index.SessionKey)
	if !ok {
		return errInvalidKey, nil
	}

	session, ok := value.(Session)
	if !ok {
		return errInvalidKey, nil
	}

	errSameFwd := `Cannot create forwarding in the same channel
Setup process is terminated`

	ret := strings.Builder{}
	// Set channel info to Session field
	switch ch := index.Index; ch {
	case firstCh:
		if session.Second == channel {
			m.sessionKeys.Delete(index.SessionKey)
			return errSameFwd, nil
		}
		session.First = channel
		fmt.Fprintf(&ret, "Successfully set this channel as: %s", session.FirstAlias)
	case secondCh:
		if session.First == channel {
			m.sessionKeys.Delete(index.SessionKey)
			return errSameFwd, nil
		}
		session.Second = channel
		fmt.Fprintf(&ret, "Successfully set this channel as: %s", session.SecondAlias)
	default:
		return "", internalError{
			msg: fmt.Sprintf("got invalid Index.Index: %v", index),
		}
	}

	if session.First.ChannelID == "" || session.Second.ChannelID == "" {
		// If the other channel is not set, prompt user to continue the process
		ret.WriteString("\nPlease set another channel with provided key")

		// Write Session back to redis
		err := m.sessionKeys.Replace(index.SessionKey, session, cache.DefaultExpiration)
		if err != nil {
			return "", internalError{
				msg: fmt.Sprintf("Store back Session error: %s", err.Error()),
			}
		}
	} else {
		// If both channels are set, remove Session from redis and create forwarding
		m.sessionKeys.Delete(index.SessionKey)

		// Twoway or oneway?
		switch cmd := session.Cmd; cmd {
		case oneWay:
			alias := Alias{
				SrcAlias: session.FirstAlias,
				DstAlias: session.SecondAlias,
			}
			ret.WriteString("\n")
			ret.WriteString(m.createFwd(session.First, session.Second, channel, alias))
		case twoWay:
			alias := Alias{
				SrcAlias: session.FirstAlias,
				DstAlias: session.SecondAlias,
			}
			ret.WriteString("\n")
			ret.WriteString(m.createFwd(session.First, session.Second, channel, alias))
			ret.WriteString("\n")
			alias = Alias{
				SrcAlias: session.SecondAlias,
				DstAlias: session.FirstAlias,
			}
			ret.WriteString(m.createFwd(session.Second, session.First, channel, alias))
		default:
			return "", internalError{
				msg: fmt.Sprintf("got invalid Cmd in Session: %v", session),
			}
		}
		m.writeToDB()
	}
	return ret.String(), nil
}
