package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// A Stream represents a twtich stream
type Stream struct {
	CommunityID  []string  `json:"community_ids"`
	GameID       string    `json:"game_id"`
	ID           string    `json:"id"`
	Language     string    `json:"language"`
	StartedAt    time.Time `json:"started_at"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	ViewerCount  int       `json:"viewer_count"`
	offline      bool
}

// A StreamList can be directly unmarshalled from twtich API respond JSON
type StreamList struct {
	Data []Stream `json:"data"`
}

type streamQuery struct {
	userID    []string
	userLogin []string
}

func (t *twitchAPI) printStream(ctx context.Context, stream Stream, userLogin string) string {
	if !stream.offline {
		game := <-t.gameByID(ctx, stream.GameID)
		var gameName string
		if game == nil {
			gameName = stream.GameID
		} else {
			gameName = game.Name
		}

		return fmt.Sprintf(`- Streamer: %s (%s)
- Title: %s
- Playing: %s
- Viewer Count: %d
- Link: %s`, stream.UserName, userLogin, stream.Title,
			gameName, stream.ViewerCount, twitchURL+userLogin)
	}
	return "offline"
}

func (t *twitchAPI) getStreams(ctx context.Context, sq streamQuery) (*StreamList, error) {
	// twitch api limits maximum query count at 100
	if len(sq.userID)+len(sq.userLogin) > 100 {
		return nil, errors.New("id + login query count exceeds limitation")
	}

	req, err := t.newRequest("GET", "streams", nil)
	if err != nil {
		return nil, err
	}

	req = req.WithContext(ctx)

	query := req.URL.Query()
	for _, login := range sq.userLogin {
		query.Add("user_login", login)
	}
	for _, id := range sq.userID {
		query.Add("user_id", id)
	}
	req.URL.RawQuery = query.Encode()

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status code: %d - %s", resp.StatusCode, string(respBody))
	}

	streams := &StreamList{}
	err = json.Unmarshal(respBody, streams)
	if err != nil {
		return nil, err
	}

	return streams, nil
}

// subscribeStream subscribes to stream changed event
// If error happened, returned channel will be closed without pushing
// Otherwise, returns nil
func (s *Service) subscribeStream(ctx context.Context, userID string) <-chan interface{} {
	logger := s.logger.WithField("phase", "subscribeStream")
	ret := make(chan interface{})
	go func() {
		defer close(ret)
		hubparams := make(url.Values)
		hubparams.Add("user_id", userID)
		err := s.subscription(ctx, "streams", &hubparams, true)
		if err != nil {
			logger.Errorf(err.Error())
			return
		}
		ret <- nil
	}()
	return ret
}

// streamChanged handles webhook callbacks for stream changed event
func (s *Service) streamChanged(request *http.Request, body []byte) int {
	logger := s.logger.WithField("phase", "streamChanged")

	userID := request.URL.Query().Get("user_id")
	chList, ok := s.subTopics["streams"].getList(userID)
	if !ok {
		// no subscribers, reply 410 to terminate the subscription
		logger.Warnf("get callback but not subscribers, do unsub. user_id: %s", userID)
		return 410
	}

	// start a goroutine for the rest of process and return 200 for this request
	go func() {
		if !ok {
			return
		}

		// Get user display name
		ctx, cancel := context.WithTimeout(s.notifCtx, reqTimeOut)
		userChan := s.api.userByID(ctx, userID)
		defer cancel()

		// Unmarshal callback body
		var streamList StreamList
		err := json.Unmarshal(body, &streamList)
		if err != nil {
			logger.Error("failed to decode request body")
			<-userChan
			return
		}

		var user *User
		select {
		case user, ok = <-userChan:
			if !ok {
				return
			} else if user == nil {
				logger.Errorf("user not found, id: %s", userID)
				return
			}
		case <-ctx.Done():
			logger.Warnf("userById timeout/cancelled")
			return
		}

		// Construct message
		var msg strings.Builder
		if len(streamList.Data) == 0 {
			fmt.Fprintf(&msg, "== Twitch Stream Offline ==\n- Streamer: %s (%s)",
				user.DisplayName, user.Login)
			s.streamStatus.Store(userID, false)
		} else {
			stream := streamList.Data[0]
			value, loaded := s.streamStatus.LoadOrStore(userID, true)
			var status bool
			if !loaded {
				status = false
			} else {
				status, _ = value.(bool)
				s.streamStatus.Store(userID, true)
			}
			if status {
				fmt.Fprintf(&msg, "== Twitch Stream Update ==\n%s",
					s.api.printStream(s.notifCtx, stream, user.Login))
			} else {
				fmt.Fprintf(&msg, "== Twitch Stream Online ==\n%s",
					s.api.printStream(s.notifCtx, stream, user.Login))
			}
		}

		// Broadcast message
		for channel := range chList {
			outMsg := telepathy.OutboundMessage{
				ToChannel: channel,
				Text:      msg.String(),
			}
			select {
			case s.msgOut <- outMsg:
			case <-s.notifCtx.Done():
				logger.Warnf("message cancelled")
				break
			}
		}
	}()

	return 200
}
