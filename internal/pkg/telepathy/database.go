package telepathy

import "github.com/sirupsen/logrus"

var databaseList map[string]DatabaseGetter
var database Database

// Database defines interfaces to backend database for telepathy
type Database interface {
	createUser(*User) error
	findUser(string) *User
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

func setDatabase(dbtype string) {
	getter := databaseList[dbtype]
	if getter == nil {
		panic("Invalid database type: " + dbtype)
	}
	database = getter()
}

func getDatabase() Database {
	return database
}
