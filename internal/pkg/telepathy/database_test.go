package telepathy

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"

	"github.com/stretchr/testify/assert"
)

const (
	testDBURL  = "mongodb://mongo:27017/test"
	testDBName = "testDBName"
)

type dbTester struct {
	handler *databaseHandler
	reqChA  chan DatabaseRequest
	reqChB  chan DatabaseRequest
	reqChC  chan DatabaseRequest
	done    chan interface{}
}

func TestDBConnection(t *testing.T) {
	assert := assert.New(t)
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if assert.NoError(err) {
		assert.NotNil(handler)
	}
	assert.NoError(handler.start(context.Background()))
}

func TestDBAttach(t *testing.T) {
	assert := assert.New(t)
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if assert.NoError(err) {
		assert.NotNil(handler)
	}
	reqCh := make(chan DatabaseRequest)
	handler.attachRequester("testReq", reqCh)
	reqChOther := make(chan DatabaseRequest)
	handler.attachRequester("testReqOther", reqChOther)
	dbDone := make(chan interface{})
	go func() {
		assert.NoError(handler.start(context.Background()))
		close(dbDone)
	}()
	close(reqCh)
	close(reqChOther)
	<-dbDone
}

func TestDBAttachDuplicate(t *testing.T) {
	assert := assert.New(t)
	handler, err := newDatabaseHandler(testDBURL, testDBName)
	if assert.NoError(err) {
		assert.NotNil(handler)
	}
	reqCh := make(chan DatabaseRequest)
	handler.attachRequester("testReq", reqCh)
	reqChOther := make(chan DatabaseRequest)
	assert.Panics(func() { handler.attachRequester("testReq", reqChOther) })
}

func (tester *dbTester) start(t *testing.T) {
	assert := assert.New(t)
	var err error
	tester.handler, err = newDatabaseHandler(testDBURL, testDBName)
	if assert.NoError(err) {
		assert.NotNil(tester.handler)
	}
	tester.reqChA = make(chan DatabaseRequest, 5)
	tester.reqChB = make(chan DatabaseRequest, 5)
	tester.reqChC = make(chan DatabaseRequest, 5)
	tester.handler.attachRequester("reqA", tester.reqChA)
	tester.handler.attachRequester("reqB", tester.reqChB)
	tester.handler.attachRequester("reqC", tester.reqChC)
	tester.done = make(chan interface{})
	go func() {
		assert.NoError(tester.handler.start(context.Background()))
		close(tester.done)
	}()
}

func (tester *dbTester) stop() {
	close(tester.reqChA)
	close(tester.reqChB)
	close(tester.reqChC)
	<-tester.done
}

func TestDBSimpleReadWrite(t *testing.T) {
	assert := assert.New(t)
	tester := dbTester{}

	testVal := "value"
	tester.start(t)
	retCh := make(chan interface{})
	tester.reqChA <- DatabaseRequest{
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
	tester.reqChB <- DatabaseRequest{
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
	tester.stop()
}

func TestDBSimpleReadWriteReconn(t *testing.T) {
	assert := assert.New(t)
	tester := dbTester{}

	testVal := "value"
	tester.start(t)
	retCh := make(chan interface{})
	tester.reqChA <- DatabaseRequest{
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

	tester.stop()

	tester.start(t)
	retCh = make(chan interface{})
	tester.reqChB <- DatabaseRequest{
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
	tester.stop()
}

func TestDBMultiAccess(t *testing.T) {
	assert := assert.New(t)
	tester := dbTester{}
	tester.start(t)
	testFunc := func(reqChan chan<- DatabaseRequest, collection string, testVal string) {
		retCh := make(chan interface{})
		reqChan <- DatabaseRequest{
			Action: func(ctx context.Context, db *mongo.Database) interface{} {
				collection := db.Collection(collection)
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
		reqChan <- DatabaseRequest{
			Action: func(ctx context.Context, db *mongo.Database) interface{} {
				collection := db.Collection(collection)
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
	}

	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		for i := 0; i < 10; i++ {
			testFunc(tester.reqChA, "collectionA", "testA")
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < 10; i++ {
			testFunc(tester.reqChB, "collectionB", "testB")
		}
		wg.Done()
	}()

	go func() {
		for i := 0; i < 10; i++ {
			testFunc(tester.reqChC, "collectionC", "testC")
		}
		wg.Done()
	}()
	wg.Wait()
	tester.stop()
}

func TestDBTimeout(t *testing.T) {
	assert := assert.New(t)
	tester := dbTester{}
	tester.start(t)
	tester.handler.timeout = 10 * time.Second
	retCh := make(chan interface{})
	tester.reqChA <- DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			<-ctx.Done()
			return true
		},
		Return: retCh,
	}
	get := <-retCh
	value, ok := get.(bool)
	assert.True(ok)
	assert.True(value)
	tester.stop()
}
