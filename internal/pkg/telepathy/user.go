package telepathy

import (
	"errors"
	"strings"
)

// User holds infomation related to a Telepathy user.
type User struct {
	ID        string            `json:"id"`
	Privilege uint              `json:"privilege"`
	MsgID     map[string]string `json:"msg_id"`
}

// MsgUser holds information related to a messenger user.
type MsgUser struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	MainID string `json:"main_id"`
}

// reservedIDPrefix cannot be prefixed to a User ID
// The IDs prefixed with reservedIDPrefix are reserved for internal use (ex. Testing)
var reservedIDPrefix = "__"

func getReservedUserID(id string) string {
	return reservedIDPrefix + id
}

// CreateUser creates and returns a new Telepathy user.
func createUser(id string) (*User, error) {
	// ID prefixed with reservedIDPrefix is reserved for internal use
	if strings.HasPrefix(id, reservedIDPrefix) {
		return nil, errors.New("Invalid User ID. User ID must not be prefixed with " + reservedIDPrefix)
	}

	db := getDatabase()

	find := db.findUser(id)
	if find != nil {
		return nil, errors.New("User with ID: " + id + " already exists.")
	}

	// Create user in database
	user := &User{ID: id, Privilege: 0}
	err := db.createUser(user)
	return user, err
}
