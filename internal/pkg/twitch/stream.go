package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/peterhellberg/link"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

// A Stream represents a twtich stream
type Stream struct {
	CommunityID  []string `json:"community_ids"`
	GameID       string   `json:"game_id"`
	ID           string   `json:"id"`
	Language     string   `json:"language"`
	StartedAt    string   `json:"started_at"`
	ThumbnailURL string   `json:"thumbnail_url"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	UserID       string   `json:"user_id"`
	UserName     string   `json:"user_name"`
	ViewerCount  int      `json:"viewer_count"`
	Online       bool     // Flag for online
}

// A StreamList can be directly unmarshalled from twtich API respond JSON
type StreamList struct {
	Data []Stream `json:"data"`
}

var subStreamLock sync.Mutex

func (t *twitchAPI) fetchStream(ctx context.Context, login string,
	respChan chan<- Stream, errChan chan<- error) {

	req, err := t.newRequest("GET", "streams", nil)
	if err != nil {
		errChan <- fmt.Errorf("new request failed: %s", err.Error())
		return
	}

	query := req.URL.Query()
	query.Add("user_login", login)
	req.URL.RawQuery = query.Encode()

	// Create go routine for http request
	getRespChan := make(chan *http.Response)
	getErrChan := make(chan error)
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go t.sendReq(getCtx, req, getRespChan, getErrChan)

	var resp *http.Response
	select {
	case resp = <-getRespChan:
		defer resp.Body.Close()
		break
	case err := <-getErrChan:
		errChan <- err
		return
	case <-ctx.Done():
		t.logger.Warn("fetchStream() timed-out / cancelled")
		errChan <- errors.New("timeout / cancelled")
		return
	}

	// Handle response
	if resp.StatusCode != 200 {
		errChan <- fmt.Errorf("fetchStream() resp status: %d (ReqURL: %s)", resp.StatusCode,
			req.URL.String())
		return
	}

	decoder := json.NewDecoder(resp.Body)
	var streamList StreamList
	err = decoder.Decode(&streamList)
	if err != nil {
		errChan <- fmt.Errorf("Resp.Body Read failed: %s", err.Error())
		return
	}

	if len(streamList.Data) == 0 {
		// Stream offline
		respChan <- Stream{}
		return
	}

	streamList.Data[0].Online = true
	respChan <- streamList.Data[0]
}

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

// streamChanged handles webhook callbacks for stream changed event
func (s *twitchService) streamChanged(ctx context.Context, request *http.Request, resp chan int) {
	localLogger := s.logger.WithField("phase", "streamChanged")

	headerLinks := link.ParseRequest(request)
	topicURL, _ := url.Parse(headerLinks["self"].URI)
	userID := topicURL.Query().Get("user_id")
	chList, ok := s.webhookSubs[whTopicStream].GetList(userID)
	if !ok {
		// no subscribers, reply 410 to terminate the subscription
		localLogger.Warnf("get callback but not subscribers, do unsub. user_id: %s", userID)
		resp <- 410
		return
	}

	// Get user display name
	var userName string
	errChan := make(chan error)
	respChan := make(chan *User)
	ctx, cancel := context.WithTimeout(s.ctx, reqTimeOut)
	defer cancel()
	go s.api.fetchUserWithID(ctx, userID, respChan, errChan)

	// Read callback body
	decoder := json.NewDecoder(request.Body)
	var streamList StreamList
	err := decoder.Decode(&streamList)
	resp <- 200
	if err != nil {
		localLogger.Error("failed to read request body")
		return
	}

	var user *User
	select {
	case user = <-respChan:
		if user == nil {
			localLogger.Warnf("user not found, id: %s", userID)
			userName = fmt.Sprintf("UserID: %s", userID)
			break
		}
		userName = user.DisplayName
	case err := <-errChan:
		localLogger.Error(err.Error())
		userName = fmt.Sprintf("UserID: %s", userID)
	case <-ctx.Done():
		localLogger.Warnf("Request timeout, please try again later.")
		userName = fmt.Sprintf("UserID: %s", userID)
	}

	// Construct message
	var msg string
	if len(streamList.Data) == 0 {
		msg = fmt.Sprintf("%s stream goes offline.", userName)
		s.streamStatus.Store(userID, false)
	} else {
		value, loaded := s.streamStatus.LoadOrStore(userID, true)
		if !loaded {
			localLogger.Warnf("no stream status when getting callback")
		}
		status, _ := value.(bool)
		if status {
			msg = "== Twitch Stream Update ==\n"
		} else {
			msg = "== Twitch Stream Online ==\n"
		}
		stream := streamList.Data[0]
		msg += fmt.Sprintf(`- Title: %s
- Streamer: %s
- Link: %s`, stream.Title, userName, twitchURL+user.Login)
	}

	// Broadcast message
	for channel := range chList {
		messenger, _ := s.session.Message.Messenger(channel.MessengerID)
		outMsg := telepathy.OutboundMessage{
			TargetID: channel.ChannelID,
			Text:     msg,
		}
		messenger.Send(&outMsg)
	}
}
