package fwd

import (
	"context"
	"fmt"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	id          = "FWD"
	funcKey     = "fwd"
	dbTableName = "fwdtable"
	outMsgLen   = 20
	dbReqLen    = 1
	redisReqLen = 1
)

// Service defines the plugin structure
type Service struct {
	telepathy.Plugin
	telepathy.PluginCommandHandler
	telepathy.PluginMsgConsumer
	telepathy.PluginMsgProducer
	telepathy.PluginDatabaseUser
	telepathy.PluginRedisUser

	inMsg    <-chan telepathy.InboundMessage
	outMsg   chan telepathy.OutboundMessage
	dbReq    chan telepathy.DatabaseRequest
	redisReq chan telepathy.RedisRequest
	cmdDone  <-chan interface{}

	table  *table
	logger *logrus.Entry
}

// ID implements telepathy.Plugin
func (m *Service) ID() string {
	return id
}

// SetLogger implements telepathy.Plugin
func (m *Service) SetLogger(logger *logrus.Entry) {
	m.logger = logger
}

// Start implements telepathy.Plugin
func (m *Service) Start() {
	m.table = newTable()

	// Starting sequence
	// 1. Load fwd table from DB
	// 2. Start table handler
	// 3. start receiving/forwarding messages
	err := m.loadFromDB()
	if err != nil {
		m.logger.Errorf("table LoadDB failed: %s", err.Error())
	}

	tableDone := make(chan interface{})
	go func() {
		m.table.start()
		close(tableDone)
	}()

	msgDone := make(chan interface{})
	go func() {
		m.msgHandler()
		close(msgDone)
	}()

	m.logger.Info("started")

	// Terminating sequence
	// 1. Wait until msgHandler and command parser ends
	// 2. close redis & close outMsg
	// 3. Store fwd table to DB
	// 4. Stop table handler
	// 5. close db
	<-msgDone
	<-m.cmdDone
	close(m.redisReq)
	close(m.outMsg)
	<-m.writeToDB()
	m.table.stop()
	<-tableDone
	close(m.dbReq)
	m.logger.Info("terminated")
}

// Stop implements telepathy.Plugin
func (m *Service) Stop() {
	// No active stop needed
}

// AttachInMsgChannel implements telepathy.PluginMsgConsumer
func (m *Service) AttachInMsgChannel(ch <-chan telepathy.InboundMessage) {
	m.inMsg = ch
}

//OutMsgChannel implements telepathy.PluginMsgProducer
func (m *Service) OutMsgChannel() <-chan telepathy.OutboundMessage {
	if m.outMsg == nil {
		m.outMsg = make(chan telepathy.OutboundMessage, outMsgLen)
	}
	return m.outMsg
}

// DBRequestChannel implements telepathy.PluginDatabaseUser
func (m *Service) DBRequestChannel() <-chan telepathy.DatabaseRequest {
	if m.dbReq == nil {
		m.dbReq = make(chan telepathy.DatabaseRequest, dbReqLen)
	}
	return m.dbReq
}

// RedisRequestChannel implements telepathy.PluginRedisUser
func (m *Service) RedisRequestChannel() <-chan telepathy.RedisRequest {
	if m.redisReq == nil {
		m.redisReq = make(chan telepathy.RedisRequest, redisReqLen)
	}
	return m.redisReq
}

func (m *Service) msgHandler() {
	for message := range m.inMsg {
		toChList := m.table.getTo(message.FromChannel)
		if toChList != nil {
			for toCh, alias := range toChList {
				outMsg := telepathy.OutboundMessage{
					ToChannel: toCh,
					AsName:    fmt.Sprintf("%s | %s", alias.SrcAlias, message.SourceProfile.DisplayName),
					Text:      message.Text,
					Image:     message.Image,
				}
				m.outMsg <- outMsg
			}
		}
	}
}

func (m *Service) writeToDB() chan interface{} {
	retCh := make(chan interface{}, 1)
	bsonChan := m.table.bson()
	m.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			tableBSON := bson.M{"ID": dbTableName, "Table": *(<-bsonChan)}
			collection := db.Collection(funcKey)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"ID": dbTableName}, tableBSON, options.Replace().SetUpsert(true))
			if err != nil {
				m.logger.Error("error when writing table back to DB: " + err.Error())
			}
			return result
		},
		Return: retCh,
	}
	return retCh
}

func (m *Service) loadFromDB() error {
	retCh := make(chan interface{}, 1)
	m.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(funcKey)
			result := collection.FindOne(ctx, map[string]string{"ID": dbTableName})
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
	return m.table.loadBSON(bsonValue)
}
