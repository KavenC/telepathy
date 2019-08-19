package telepathy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

const (
	testDBURL  = "mongodb://mongo:27017/test"
	testDBName = "testDBName"
)

type DatabaseSetupTestSuite struct {
	suite.Suite
}

type DatabaseTestSuite struct {
	suite.Suite
	handler *databaseHandler
}

func (setupSuite *DatabaseSetupTestSuite) TestConnection() {
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if setupSuite.NoError(err) {
		setupSuite.NotNil(handler)
	}
	setupSuite.NoError(handler.start(context.Background()))
}

func (setupSuite *DatabaseSetupTestSuite) TestAttach() {
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if setupSuite.NoError(err) {
		setupSuite.NotNil(handler)
	}
	reqCh := make(chan DatabaseRequest)
	handler.attachRequester("testReq", reqCh)
	reqChOther := make(chan DatabaseRequest)
	handler.attachRequester("testReqOther", reqChOther)
	dbDone := make(chan interface{})
	go func() {
		setupSuite.NoError(handler.start(context.Background()))
		close(dbDone)
	}()
	close(reqCh)
	close(reqChOther)
	<-dbDone
}

func (setupSuite *DatabaseSetupTestSuite) TestAttachDuplicate() {
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if setupSuite.NoError(err) {
		setupSuite.NotNil(handler)
	}
	reqCh := make(chan DatabaseRequest)
	handler.attachRequester("testReq", reqCh)
	reqChOther := make(chan DatabaseRequest)
	setupSuite.Panics(func() { handler.attachRequester("testReq", reqChOther) })
}

func TestDatabaseSetup(t *testing.T) {
	suite.Run(t, new(DatabaseSetupTestSuite))
}
