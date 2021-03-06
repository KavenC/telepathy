package telepathy_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"gitlab.com/kavenc/argo"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

type testMessenger struct {
	id            string
	webhooks      map[string]telepathy.HTTPHandler
	urls          map[string]*url.URL
	inMsgChannel  chan telepathy.InboundMessage
	outMsgChannel <-chan telepathy.OutboundMessage
}
type testService struct {
	id            string
	cmd           *argo.Action
	inMsgChannel  <-chan telepathy.InboundMessage
	outMsgChannel chan telepathy.OutboundMessage
	dbChannel     chan telepathy.DatabaseRequest
	cmdDone       <-chan interface{}
}

func (m *testMessenger) ID() string {
	return m.id
}

func (m *testMessenger) SetLogger(_ *logrus.Entry) {}

func (m *testMessenger) Start() {}

func (m *testMessenger) Stop() {
	close(m.inMsgChannel)
}

func (m *testMessenger) InMsgChannel() <-chan telepathy.InboundMessage {
	m.inMsgChannel = make(chan telepathy.InboundMessage)
	return m.inMsgChannel
}

func (m *testMessenger) AttachOutMsgChannel(ch <-chan telepathy.OutboundMessage) {
	m.outMsgChannel = ch
}

func (m *testMessenger) Webhook() map[string]telepathy.HTTPHandler {
	return m.webhooks
}

func (m *testMessenger) SetWebhookURL(urls map[string]*url.URL) {
	m.urls = make(map[string]*url.URL)
	for endpoint, url := range urls {
		m.urls[endpoint] = url
	}
}

func (s *testService) ID() string {
	return s.id
}

func (s *testService) SetLogger(_ *logrus.Entry) {}

func (s *testService) Start() {
	<-s.cmdDone
	close(s.outMsgChannel)
	close(s.dbChannel)
}

func (s *testService) Stop() {
}

func (s *testService) Command(done <-chan interface{}) *argo.Action {
	s.cmdDone = done
	return s.cmd
}

func (s *testService) AttachInMsgChannel(ch <-chan telepathy.InboundMessage) {
	s.inMsgChannel = ch
}

func (s *testService) OutMsgChannel() <-chan telepathy.OutboundMessage {
	s.outMsgChannel = make(chan telepathy.OutboundMessage)
	return s.outMsgChannel
}

func (s *testService) DBRequestChannel() <-chan telepathy.DatabaseRequest {
	s.dbChannel = make(chan telepathy.DatabaseRequest)
	return s.dbChannel
}

func newSessionConfig() *telepathy.SessionConfig {
	return &telepathy.SessionConfig{
		Port:         "80",
		RootURL:      "http://localhost",
		MongoURL:     "mongodb://mongo:27017/test",
		DatabaseName: "SessionTest",
	}
}

func TestSessionIntegration(t *testing.T) {
	assert := assert.New(t)
	config := newSessionConfig()
	pluginMsgr := testMessenger{id: "MSGR"}
	pluginMsgr.webhooks = make(map[string]telepathy.HTTPHandler)
	fromCh := telepathy.Channel{
		MessengerID: "MSGR",
		ChannelID:   "WEBHOOK",
	}
	pluginMsgr.webhooks["test-hook"] = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		bodyByte, err := ioutil.ReadAll(req.Body)
		req.Body.Close()
		assert.NoError(err)
		pluginMsgr.inMsgChannel <- telepathy.InboundMessage{
			FromChannel: fromCh,
			Text:        string(bodyByte),
		}
	}

	pluginSvc := testService{id: "SVC"}
	cmd := argo.Action{Trigger: "svc"}
	cmd.AddSubAction(argo.Action{
		Trigger: "act",
		Do: func(state *argo.State, extraArgs ...interface{}) error {
			state.OutputStr.WriteString("success")
			return nil
		},
	})
	pluginSvc.cmd = &cmd

	session, err := telepathy.NewSession(*config,
		[]telepathy.Plugin{&pluginMsgr, &pluginSvc})
	assert.NoError(err)

	done := make(chan interface{})
	go func() {
		session.Start(context.Background())
		close(done)
	}()

	// Webhook -> Msgr -> Cmd -> SVC -> OutMsg
	resp, err := http.Post(pluginMsgr.urls["test-hook"].String(), "text/plain",
		strings.NewReader("teru svc act"))
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)

	msg := <-pluginMsgr.outMsgChannel
	assert.Equal("success", msg.Text)

	// Webhook -> Msgr -> SVC -> OutMsg
	resp, err = http.Post(pluginMsgr.urls["test-hook"].String(), "text/plain",
		strings.NewReader("random text"))
	assert.NoError(err)
	assert.Equal(200, resp.StatusCode)
	inMsg := <-pluginSvc.inMsgChannel
	assert.Equal("random text", inMsg.Text)
	assert.Equal(fromCh, inMsg.FromChannel)

	// DB Req
	testVal := "dbTest"
	retCh := make(chan interface{})
	pluginSvc.dbChannel <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection("testCollection")
			_, err := collection.InsertOne(ctx, bson.M{"Key": testVal})
			assert.NoError(err)
			return testVal
		},
		Return: retCh,
	}
	ret := <-retCh
	close(retCh)
	value, ok := ret.(string)
	assert.True(ok)

	retCh = make(chan interface{})
	pluginSvc.dbChannel <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection("testCollection")
			result := collection.FindOne(ctx, map[string]string{"Key": value})
			if !assert.NoError(result.Err()) {
				return ""
			}
			raw, err := result.DecodeBytes()
			if !assert.NoError(err) {
				return ""
			}
			readValue, err := raw.LookupErr("Key")
			if !assert.NoError(err) {
				return ""
			}
			ret, ok := readValue.StringValueOK()
			if !assert.True(ok) {
				return ""
			}
			delResult, err := collection.DeleteMany(ctx, map[string]string{"Key": value})
			assert.NoError(err)
			assert.Equal(int64(1), delResult.DeletedCount)
			return ret
		},
		Return: retCh,
	}

	ret = <-retCh
	close(retCh)
	value, ok = ret.(string)
	assert.True(ok)
	assert.Equal(testVal, value)

	session.Stop()
	<-done
}

func TestSessionCtxDatabase(t *testing.T) {
	assert := assert.New(t)
	config := newSessionConfig()
	pluginMsgr := testMessenger{id: "MSGR"}
	pluginSvc := testService{id: "SVC"}
	cmd := argo.Action{Trigger: "svc"}
	pluginSvc.cmd = &cmd

	session, err := telepathy.NewSession(*config,
		[]telepathy.Plugin{&pluginSvc, &pluginMsgr})
	assert.NoError(err)

	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		session.Start(ctx)
		wg.Done()
	}()

	retCh := make(chan interface{})
	pluginSvc.dbChannel <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			<-ctx.Done()
			return "done"
		},
		Return: retCh,
	}

	go func() {
		ret := <-retCh
		close(retCh)
		value, _ := ret.(string)
		assert.Equal("done", value)
		wg.Done()
	}()
	cancel()

	session.Stop()
	wg.Wait()
}

func TestSessionCtxCmd(t *testing.T) {
	assert := assert.New(t)
	config := newSessionConfig()
	pluginMsgr := testMessenger{id: "MSGR"}
	pluginSvc := testService{id: "SVC"}
	cmd := argo.Action{Trigger: "svc"}
	cmd.AddSubAction(argo.Action{
		Trigger: "hang",
		Do: func(state *argo.State, extraArgs ...interface{}) error {
			extra, _ := extraArgs[0].(telepathy.CmdExtraArgs)
			<-extra.Ctx.Done()
			return nil
		},
	})
	pluginSvc.cmd = &cmd

	session, err := telepathy.NewSession(*config,
		[]telepathy.Plugin{&pluginSvc, &pluginMsgr})
	assert.NoError(err)

	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(2)
	go func() {
		session.Start(ctx)
		wg.Done()
	}()

	pluginMsgr.inMsgChannel <- telepathy.InboundMessage{
		FromChannel: telepathy.Channel{
			MessengerID: "MSGR",
		},
		Text: "teru svc hang",
	}

	go func() {
		<-pluginMsgr.outMsgChannel
		wg.Done()
	}()
	cancel()

	session.Stop()
	wg.Wait()
}
