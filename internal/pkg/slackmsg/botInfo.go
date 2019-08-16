package slackmsg

import (
	"context"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	dbID         = "slackBotInfo"
	collectionID = "slack"
)

type botInfo struct {
	BotID       string
	BotUserID   string
	AccessToken string
}

type botInfoMap map[string]botInfo

func (m *Messenger) writeBotInfoToDB() chan interface{} {
	ret := make(chan interface{}, 1)

	m.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			data := bson.M{
				"ID":   dbID,
				"Info": m.botInfoMap,
			}
			collection := db.Collection(collectionID)
			result, err := collection.ReplaceOne(ctx,
				map[string]string{"ID": dbID}, data, options.Replace().SetUpsert(true))
			if err != nil {
				m.logger.Error("error when writing table back to DB: " + err.Error())
			}
			return result
		},
		Return: ret,
	}

	return ret
}

func (m *Messenger) readBotInfoFromDB() error {
	ret := make(chan interface{}, 1)

	m.dbReq <- telepathy.DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection(collectionID)
			result := collection.FindOne(ctx, map[string]string{"ID": dbID})
			raw, err := result.DecodeBytes()
			if err != nil {
				return err
			}
			return raw
		},
		Return: ret,
	}

	// Wait until DB operation done
	result := <-ret
	if err, ok := result.(error); ok {
		return err
	}

	raw, _ := result.(bson.Raw)
	err := raw.Lookup("Info").Unmarshal(&m.botInfoMap)
	if err != nil {
		return err
	}
	return nil
}
