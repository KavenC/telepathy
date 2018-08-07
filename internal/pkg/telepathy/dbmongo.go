package telepathy

import (
	"context"
	"os"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/sirupsen/logrus"
)

type mongoDatabase struct {
	database *mongo.Database
}

type mongoUser struct {
	_id string
	*User
}

var userCollection = "User"

func init() {
	RegisterDatabase("mongo", getter)
	return
}

func getter() Database {
	database := &mongoDatabase{}
	err := database.connect()
	if err != nil {
		logrus.Error("MongoDB connection failed.")
		panic(err)
	}
	return database
}

func (m *mongoDatabase) connect() error {
	if m.database == nil {
		client, err := mongo.NewClient(os.Getenv("MONGODB_URL"))
		if err != nil {
			return err
		}
		err = client.Connect(context.TODO())
		if err != nil {
			return err
		}
		m.database = client.Database(os.Getenv("MONGODB_NAME"))
	}

	return nil
}

// CreateUser creates a user entry in mongo db
func (m *mongoDatabase) createUser(ctx context.Context, user *User) error {
	collection := m.database.Collection(userCollection)
	mongouser := mongoUser{
		_id:  user.ID,
		User: user,
	}
	_, err := collection.InsertOne(ctx, mongouser)
	return err
}

// FindUser find user with ID in mongo db
func (m *mongoDatabase) findUser(ctx context.Context, id string) *User {
	collection := m.database.Collection(userCollection)
	result := collection.FindOne(ctx, map[string]string{"_id": id})
	if result == nil {
		return nil
	}

	user := mongoUser{}
	result.Decode(user)

	return user.User
}
