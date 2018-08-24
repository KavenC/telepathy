package fwd

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func TestTableBSON(t *testing.T) {
	table := &sync.Map{}
	from := telepathy.Channel{
		MessengerID: "MSG_1",
		ChannelID:   "CID_1",
	}
	to := telepathy.Channel{
		MessengerID: "MSG_2",
		ChannelID:   "CID_2",
	}
	to2 := telepathy.Channel{
		MessengerID: "MSG_3",
		ChannelID:   "CID_3",
	}
	createFwdNoLock(&from, &to, table)
	createFwdNoLock(&from, &to2, table)
	doc := tableToBSON(table)
	newTable := bsonToTable(doc)

	table.Range(func(k, v interface{}) bool {
		nv, ok := newTable.Load(k)
		if !ok {
			ch, _ := k.(telepathy.Channel)
			fmt.Println("From not found: " + ch.Name())
			t.Fail()
		}
		if !reflect.DeepEqual(v, nv) {
			t.Fail()
		}
		return true
	})
	newTable.Range(func(k, v interface{}) bool {
		nv, ok := table.Load(k)
		if !ok {
			ch, _ := k.(telepathy.Channel)
			fmt.Println("From not found: " + ch.Name())
			t.Fail()
		}
		if !reflect.DeepEqual(v, nv) {
			t.Fail()
		}
		return true
	})
}
