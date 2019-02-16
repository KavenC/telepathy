package telepathy

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/mongodb/mongo-go-driver/mongo"
)

func printCLM(clm *ChannelListMap) {
	clm.Range(func(key interface{}, chList ChannelList) bool {
		fmt.Printf("  %s: ", key)
		for ch := range chList {
			fmt.Printf("%s, ", ch.JSON())
		}
		fmt.Printf("\n")
		return true
	})
}

func clmIncludes(clmA, clmB *ChannelListMap) bool {
	ret := true
	clmA.Range(func(key interface{}, chList ChannelList) bool {
		bValue, ok := clmB.GetList(key)
		if !ok || !reflect.DeepEqual(bValue, chList) {
			ret = false
			return false
		}
		return true
	})
	return ret
}

func clmEqual(clmA, clmB *ChannelListMap) bool {
	return clmIncludes(clmA, clmB) && clmIncludes(clmA, clmB)
}

func clmTestLoadStoreBase(clm *ChannelListMap, t *testing.T) {
	dbhandler, err := newDatabaseHandler(os.Getenv("MONGODB_URL"), os.Getenv("MONGODB_NAME"))
	if err != nil {
		t.Errorf("database error: %s", err.Error())
	}
	dbhandler.start(context.Background())
	retCh := make(chan interface{}, 1)
	dbhandler.PushRequest(&DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection("clm")
			result, err := clm.StoreToDB(ctx, collection, "clm_test")
			if err != nil {
				return err
			}
			return result
		},
		Return: retCh,
	})
	ret := <-retCh
	err, ok := ret.(error)
	if ok {
		t.Errorf("store to db error: %s", err.Error())
	}

	var keyType reflect.Type
	clm.Range(func(key interface{}, _ ChannelList) bool {
		keyType = reflect.TypeOf(key)
		return false
	})

	loadClm := NewChannelListMap()
	dbhandler.PushRequest(&DatabaseRequest{
		Action: func(ctx context.Context, db *mongo.Database) interface{} {
			collection := db.Collection("clm")
			err := loadClm.LoadFromDB(ctx, collection, "clm_test", keyType)
			return err
		},
		Return: retCh,
	})
	ret = <-retCh
	err, ok = ret.(error)
	if ok && err != nil {
		t.Errorf("load from db error: %s", err.Error())
	}

	if !clmEqual(clm, loadClm) {
		fmt.Printf("Unmatched content\n")
		fmt.Printf("clm:\n")
		printCLM(clm)
		fmt.Printf("\n")
		fmt.Printf("loadClm:\n")
		printCLM(loadClm)
		t.FailNow()
	}
}

func TestCLMLoadStoreStrKey(t *testing.T) {
	clm := NewChannelListMap()
	clm.setList("testStr", ChannelList{
		Channel{MessengerID: "msg", ChannelID: "channel"}:   true,
		Channel{MessengerID: "msg2", ChannelID: "channel2"}: true,
	})
	clm.setList("testStr2", ChannelList{
		Channel{MessengerID: "msg", ChannelID: "channel"}:   true,
		Channel{MessengerID: "msg2", ChannelID: "channel2"}: true,
	})
	clmTestLoadStoreBase(clm, t)
}

func TestCLMLoadStoreChannelKey(t *testing.T) {
	clm := NewChannelListMap()
	clm.setList(Channel{"key", "keych"}, ChannelList{
		Channel{MessengerID: "msg", ChannelID: "channel"}:   true,
		Channel{MessengerID: "msg2", ChannelID: "channel2"}: true,
	})
	clm.setList(Channel{"key2", "keych2"}, ChannelList{
		Channel{MessengerID: "msg", ChannelID: "channel"}:   true,
		Channel{MessengerID: "msg2", ChannelID: "channel2"}: true,
	})
	clmTestLoadStoreBase(clm, t)
}

func TestCLMAddGet(t *testing.T) {
	clm := NewChannelListMap()
	ch := Channel{MessengerID: "msg", ChannelID: "ch"}
	clm.AddChannel("key", ch)
	list, ok := clm.GetList("key")
	if !ok || !list[ch] || len(list) != 1 {
		t.FailNow()
	}
}

func TestCLMGetCopy(t *testing.T) {
	clm := NewChannelListMap()
	ch := Channel{MessengerID: "msg", ChannelID: "ch"}
	clm.AddChannel("key", ch)
	list, ok := clm.GetList("key")
	ch2 := Channel{MessengerID: "msg2", ChannelID: "ch2"}
	clm.AddChannel("key", ch2)
	if !ok || !list[ch] || len(list) != 1 {
		t.FailNow()
	}
}

func TestCLMBasicAdd(t *testing.T) {
	clm := NewChannelListMap()
	ch := Channel{MessengerID: "msg", ChannelID: "ch"}
	ok := clm.AddChannel("key", ch)
	if !ok {
		t.FailNow()
	}
	ok = clm.AddChannel("key", ch)
	if ok {
		t.FailNow()
	}
}

func TestCLMAddDel(t *testing.T) {
	clm := NewChannelListMap()
	ch := Channel{MessengerID: "msg", ChannelID: "ch"}
	ok := clm.DelChannel("key", ch)
	if ok {
		t.FailNow()
	}
	clm.AddChannel("key", ch)
	ok = clm.DelChannel("key", ch)
	if !ok {
		t.FailNow()
	}
}

func TestCLMMultiWriter(t *testing.T) {
	clm := NewChannelListMap()
	wg := sync.WaitGroup{}
	count := 5

	wg.Add(count)
	addFunc := func(index int) {
		ch := Channel{MessengerID: fmt.Sprintf("msg%d", index), ChannelID: fmt.Sprintf("ch%d", index)}
		clm.AddChannel("key", ch)
		wg.Done()
	}

	for i := 0; i < count; i++ {
		go addFunc(i)
	}
	wg.Wait()

	list, ok := clm.GetList("key")
	if !ok || len(list) != count {
		t.FailNow()
	}

	for i := 0; i < count; i++ {
		ch := Channel{MessengerID: fmt.Sprintf("msg%d", i), ChannelID: fmt.Sprintf("ch%d", i)}
		if !list[ch] {
			t.FailNow()
		}
	}
}
