package telepathy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	testDBURL  = "mongodb://mongo:27017/test"
	testDBName = "testDBName"
)

type DatabaseTestSuite struct {
	suite.Suite
	handler *databaseHandler
}

func TestDatabaseConnection(t *testing.T) {
	assert := assert.New(t)
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if assert.NoError(err) {
		assert.NotNil(handler)
	}
	assert.NoError(handler.start())
}
