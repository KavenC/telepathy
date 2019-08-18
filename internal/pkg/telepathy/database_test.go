package telepathy

import "testing"

const (
	testDBURL  = "mongodb://localhost:27017"
	testDBName = "testDBName"
)

func TestDatabaseConn(t *testing.T) {
	_, err := newDatabaseHandler(testDBURL, testDBName)
	if err != nil {
		t.Errorf(err.Error())
	}
}
