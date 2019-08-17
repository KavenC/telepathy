// Package twitch provides twtitch.tv services for package telepathy
// Needed  Config
// - CLIENT_ID: The client ID of twitch api
package twitch

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	twitchURL = "https://www.twitch.tv/"
)

type notification struct {
	request *http.Request
	body    []byte
	status  chan int
}

// Service defines telepathy plugin
type Service struct {
	telepathy.Plugin
	telepathy.PluginCommandHandler
	telepathy.PluginWebhookHandler
	telepathy.PluginMsgProducer
	telepathy.PluginDatabaseUser

	cmdDone <-chan interface{}
	msgOut  chan telepathy.OutboundMessage
	dbReq   chan telepathy.DatabaseRequest

	webhookURL *url.URL

	api *twitchAPI

	subTopics     map[string]*table // topic -> user id -> [channels]
	verifyingSubs sync.Map

	// Notification handling routine
	notifQueue  chan *notification
	notifCtx    context.Context
	notifCancel context.CancelFunc
	notifDone   chan interface{}

	// Sub renew routine
	renewCtx       context.Context // context controls all websub renewal routines
	renewCancel    context.CancelFunc
	renewCancelMap sync.Map

	streamStatus sync.Map // UserID -> stream status

	// HMAC secret for validating incoming notifications
	WebsubSecret []byte

	// The client ID of twitch API
	ClientID string

	logger *logrus.Entry
}

// ID implements telepathy.Plugin interface
func (s *Service) ID() string {
	return "TWITCH"
}

// SetLogger implements telepathy.Plugin interface
func (s *Service) SetLogger(logger *logrus.Entry) {
	s.logger = logger
}

// Start implements telepathy.Plugin interface
func (s *Service) Start() {
	// Initialize
	s.notifQueue = make(chan *notification, 10)
	s.notifCtx, s.notifCancel = context.WithCancel(context.Background())
	s.notifDone = make(chan interface{})

	s.renewCtx, s.renewCancel = context.WithCancel(context.Background())

	s.api = newTwitchAPI()
	s.api.clientID = s.ClientID
	s.api.websubSecret = string(s.WebsubSecret)
	s.api.webhookURL = s.webhookURL
	s.api.logger = s.logger.WithField("module", "api")

	s.subTopics = make(map[string]*table)

	// - Supported Topics
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), reqTimeOut)
	s.subTopics["streams"] = newTable()
	s.loadFromDB("streams")
	for _, userID := range s.subTopics["streams"].getKeys() {
		wg.Add(1)
		go func(id string) {
			<-s.subscribeStream(ctx, id)
			wg.Done()
		}(userID)
	}
	wg.Wait()
	cancel()

	go s.notifHandler()

	s.logger.Info("started")
	// Wait for close
	<-s.cmdDone

	// write back db
	dbDone := s.writeToDB("streams")

	// Cancel all websub renewal routines
	s.renewCancel()

	// Terminate websub handling
	close(s.notifQueue)
	<-s.notifDone
	close(s.msgOut)

	// wait for writeback
	<-dbDone
	close(s.dbReq)

	s.logger.Info("terminated")
}

// Stop implements telepathy.Plugin interface
func (s *Service) Stop() {

}

// OutMsgChannel implements telepathy.PluginMsgProducer
func (s *Service) OutMsgChannel() <-chan telepathy.OutboundMessage {
	if s.msgOut == nil {
		s.msgOut = make(chan telepathy.OutboundMessage, 10)
	}
	return s.msgOut
}

// DBRequestChannel implements telepathy.PluginDatabaseUser
func (s *Service) DBRequestChannel() <-chan telepathy.DatabaseRequest {
	if s.dbReq == nil {
		s.dbReq = make(chan telepathy.DatabaseRequest, 1)
	}
	return s.dbReq
}

func (s *Service) writeToDB(topic string) chan interface{} {
	logger := s.logger.WithField("phase", "writeToDB")
	retCh := make(chan interface{}, 1)
	tableBSON := s.subTopics[topic].bson()
	s.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			dbBSON := bson.M{"ID": topic, "Table": *tableBSON}
			collection := db.Collection("twitch")
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"ID": topic}, dbBSON, options.Replace().SetUpsert(true))
			if err != nil {
				logger.Error("error when writing table back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	}
	return retCh
}

func (s *Service) loadFromDB(topic string) error {
	retCh := make(chan interface{}, 1)
	s.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection("twitch")
			result := collection.FindOne(ctx, map[string]string{"ID": topic})
			raw, err := result.DecodeBytes()
			if err != nil {
				return err
			}
			return raw.Lookup("Table")
		},
		Return: retCh,
	}

	// Wait until DB operation is done
	result := <-retCh
	if err, ok := result.(error); ok {
		return err
	}

	bsonValue, _ := result.(bson.RawValue)
	return s.subTopics[topic].fromBSON(bsonValue)
}
