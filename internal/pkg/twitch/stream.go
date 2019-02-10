package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/replaceopt"
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

func tableToBSON(table map[string]map[telepathy.Channel]bool) *bson.Document {
	doc := bson.NewDocument(bson.EC.String("topic", whTopicStream))
	for userid, channelList := range table {
		if len(channelList) == 0 {
			continue
		}
		bsonList := bson.NewArray()
		for channel := range channelList {
			bsonList.Append(bson.VC.String(channel.JSON()))
		}
		doc.Append(bson.EC.Array(userid, bsonList))
	}
	return doc
}

func bsonToTable(doc *bson.Document) map[string]map[telepathy.Channel]bool {
	table := make(map[string]map[telepathy.Channel]bool)
	eleIter := doc.Iterator()
	for {
		if !eleIter.Next() {
			break
		}
		element := eleIter.Element()
		toList, ok := element.Value().MutableArrayOK()
		if ok {
			userid := element.Key()
			chIter, _ := toList.Iterator()
			table[userid] = make(map[telepathy.Channel]bool)
			for {
				if !chIter.Next() {
					break
				}
				channel := &telepathy.Channel{}
				jsonStr := chIter.Value().StringValue()
				json.Unmarshal([]byte(jsonStr), channel)
				table[userid][*channel] = true
			}
		}
	}
	return table
}

func (s *twitchService) streamChangeWriteToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	s.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(ID)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"topic": whTopicStream}, tableToBSON(s.webhookSubs[whTopicStream]),
				replaceopt.Upsert(true))
			if err != nil {
				s.logger.Error("error when write-back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (s *twitchService) streamChangeLoadFromDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	s.session.DB.PushRequest(&telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(ID)
			result := collection.FindOne(ctx, map[string]string{"topic": whTopicStream})
			doc := bson.NewDocument()
			err := result.Decode(doc)
			table := bsonToTable(doc)
			if err != nil {
				s.logger.Error("error when load from DB: " + err.Error())
			} else {
				s.webhookSubs[whTopicStream] = table
				for userid := range s.webhookSubs[whTopicStream] {
					err := s.subscribeStream(userid)
					if err != nil {
						s.logger.WithField("phase", "loadFromDB").Errorf("subscribe stream failed: %s", err.Error())
					}
				}
			}
			s.logger.Info("load from DB done")
			return result
		},
		Return: retCh,
	})
	return retCh
}

func (s *twitchService) streamChangedDel(userid string, channel telepathy.Channel) (bool, error) {
	if s.webhookSubs[whTopicStream] == nil || s.webhookSubs[whTopicStream][userid] == nil ||
		!s.webhookSubs[whTopicStream][userid][channel] {
		return false, nil
	}

	delete(s.webhookSubs[whTopicStream][userid], channel)
	if len(s.webhookSubs[whTopicStream][userid]) == 0 {
		delete(s.webhookSubs[whTopicStream], userid)
	}
	if len(s.webhookSubs[whTopicStream]) == 0 {
		delete(s.webhookSubs, whTopicStream)
	}

	// We don't explicitly send unsubscribe request to hub. The subscription will be either expired or rejected at next
	// callback
	s.streamChangeWriteToDB()

	return true, nil
}

func (s *twitchService) subscribeStream(userid string) error {
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
			if s.webhookSubs[whTopicStream] == nil || s.webhookSubs[whTopicStream][userid] == nil {
				return
			}
			s.subscribeStream(userid)
		}()
	}()

	return nil
}

func (s *twitchService) streamChangedAdd(userid string, channel telepathy.Channel) (bool, error) {
	if s.webhookSubs[whTopicStream] == nil {
		s.webhookSubs[whTopicStream] = make(map[string]map[telepathy.Channel]bool)
	}

	if s.webhookSubs[whTopicStream][userid] == nil {
		s.webhookSubs[whTopicStream][userid] = make(map[telepathy.Channel]bool)
		err := s.subscribeStream(userid)
		if err != nil {
			return false, err
		}
	}

	if s.webhookSubs[whTopicStream][userid][channel] {
		return false, nil
	}

	s.webhookSubs[whTopicStream][userid][channel] = true
	s.streamChangeWriteToDB()

	return true, nil
}

// streamChanged handles webhook callbacks for stream changed event
func (s *twitchService) streamChanged(ctx context.Context, request *http.Request, resp chan int) {
	localLogger := s.logger.WithField("phase", "streamChanged")

	headerLinks := link.ParseRequest(request)
	topicURL, _ := url.Parse(headerLinks["self"].URI)
	userID := topicURL.Query().Get("user_id")
	if s.webhookSubs[whTopicStream] == nil || s.webhookSubs[whTopicStream][userID] == nil {
		// no subscribers, reply 410 to terminate the subscription
		localLogger.Warnf("get callback but not subscribers, do unsub. ReqURL: %s", request.URL.String())
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
		s.streamStatus[userID] = false
	} else {
		if s.streamStatus[userID] {
			msg = "== Twitch Stream Update ==\n"
		} else {
			msg = "== Twitch Stream Online ==\n"
		}
		s.streamStatus[userID] = true
		stream := streamList.Data[0]
		msg += fmt.Sprintf(`- Title: %s
- Streamer: %s
- Link: %s`, stream.Title, userName, twitchURL+user.Login)
	}

	// Broadcast message
	for channel := range s.webhookSubs[whTopicStream][userID] {
		messenger, _ := s.session.Message.Messenger(channel.MessengerID)
		outMsg := telepathy.OutboundMessage{
			TargetID: channel.ChannelID,
			Text:     msg,
		}
		messenger.Send(&outMsg)
	}
}
