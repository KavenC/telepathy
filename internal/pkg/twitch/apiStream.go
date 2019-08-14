package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"time"
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

/*
var subStreamLock sync.Mutex
func (s *twitchService) streamChangeWriteToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	s.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(ID)
			result, err := s.webhookSubs[whTopicStream].StoreToDB(ctx, collection, whTopicStream)
			if err != nil {
				s.logger.Error("error when store to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (s *twitchService) streamChangeLoadFromDB() {
	retCh := make(chan interface{}, 1)
	s.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			s.webhookSubs[whTopicStream] = telepathy.NewChannelListMap()
			collection := db.Collection(ID)
			err := s.webhookSubs[whTopicStream].LoadFromDB(ctx, collection, whTopicStream, reflect.TypeOf(""))
			if err != nil {
				s.logger.Error("error when load from DB: " + err.Error())
			}
			s.logger.Info("load from DB done")
			return err
		},
		Return: retCh,
	})
	<-retCh

	// After load from db, resubscribe to websub topic
	s.webhookSubs[whTopicStream].Range(func(key interface{}, _ telepathy.ChannelList) bool {
		userid, _ := key.(string)
		err := s.subscribeStream(userid)
		if err != nil {
			s.logger.WithField("phase", "loadFromDB").Errorf("subscribe stream failed: %s", err.Error())
		}
		return true
	})
	return
}

func (s *twitchService) streamChangedDel(userid string, channel telepathy.Channel) (bool, error) {
	if !s.webhookSubs[whTopicStream].DelChannel(userid, channel) {
		return false, nil
	}

	if !s.webhookSubs[whTopicStream].KeyExists(userid) {
		s.streamStatus.Delete(userid)
	}

	// We don't explicitly send unsubscribe request to hub. The subscription will be either expired or rejected at next
	// callback
	s.streamChangeWriteToDB()

	return true, nil
}

func (s *twitchService) subscribeStream(userid string) error {
	subStreamLock.Lock()
	defer subStreamLock.Unlock()
	_, loaded := s.streamStatus.LoadOrStore(userid, false)
	if loaded {
		// already subscribed
		return nil
	}

	// Send subscription request to twitch websub hub
	webSubHub := s.api.newWebSubHub()
	webSubHub.Set("hub.mode", "subscribe")

	callbackURL, _ := url.Parse(s.webhookURL.String())
	query := callbackURL.Query()
	query.Set("topic", whTopicStream)
	callbackURL.RawQuery = query.Encode()
	webSubHub.Set("hub.callback", callbackURL.String())

	topicURL, err := newWebhookTopicURL(whTopicStream)
	if err != nil {
		return err
	}
	query = topicURL.Query()
	query.Set("user_id", userid)
	topicURL.RawQuery = query.Encode()
	webSubHub.Set("hub.topic", topicURL.String())

	// Do websub sub process
	go func() {
		lease := s.api.requestToHub(s.ctx, webSubHub)

		// create update function
		go func() {
			sleepSec := lease - 60
			if sleepSec <= 0 {
				s.logger.Warnf("Short lease: %d, skipping websub update.", lease)
				return
			}
			time.Sleep(time.Duration(sleepSec) * time.Second)
			s.subscribeStream(userid)
		}()
	}()

	return nil
}

func (s *twitchService) streamChangedAdd(userid string, channel telepathy.Channel) (bool, error) {
	added := s.webhookSubs[whTopicStream].AddChannel(userid, channel)
	if !added {
		return false, nil
	}

	err := s.subscribeStream(userid)
	if err != nil {
		s.webhookSubs[whTopicStream].DelChannel(userid, channel)
		return false, err
	}

	s.streamChangeWriteToDB()
	return true, nil
}


*/
