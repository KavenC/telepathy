package telepathy

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

var databaseList map[string]DatabaseGetter
var database Database

// Database defines interfaces to backend database for telepathy
type Database interface {
	createUser(context.Context, *User) error
	findUser(context.Context, string) *User
}

// DatabaseGetter defines the function used to get a pointer of Database implementation
type DatabaseGetter func() Database

// RegisterDatabase is used to register a new Database
func RegisterDatabase(name string, getter DatabaseGetter) {
	if databaseList == nil {
		databaseList = make(map[string]DatabaseGetter)
	}

	logrus.Info("Regsitering Database: " + name)
	if databaseList[name] != nil {
		panic("Database with name: " + name + " already exists.")
	}
	databaseList[name] = getter
}

func initDatabase(dbtype string) error {
	getter := databaseList[dbtype]
	if getter == nil {
		return errors.New("Invalid database type: " + dbtype)
	}
	database = getter()

	return nil
}

func getDatabase() Database {
	return database
}
