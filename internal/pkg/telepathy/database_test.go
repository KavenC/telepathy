package telepathy

import (
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
	setupSuite.NoError(handler.start())
}

func (setupSuite *DatabaseSetupTestSuite) TestAttach() {
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if setupSuite.NoError(err) {
		setupSuite.NotNil(handler)
	}
}

func TestDatabaseSetup(t *testing.T) {
	suite.Run(t, new(DatabaseSetupTestSuite))
}
