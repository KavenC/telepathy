package fwd

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

func getTestTable() (*table, func()) {
	tab := table{logger: logrus.WithField("module", "table_test")}
	ctx, cancel := context.WithCancel(context.Background())
	tab.start(ctx)
	return &tab, cancel
}

func TestTableInsert(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	ret := <-tab.insert(from, to)
	if !ret.ok {
		t.Error("insert failed")
	}
	if to.Alias != ret.Alias {
		t.Errorf("alias failed: %s", ret.Alias)
	}
}

func TestTableInsertDuplicate(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)

	to.Alias = Alias{SrcAlias: "src1", DstAlias: "dst1"}

	ret := <-tab.insert(from, to)
	if ret.ok {
		t.Error("insert duplicated failed")
	}
}

func TestTableDelete(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)

	ret := <-tab.delete(from, to)
	if !ret {
		t.Error("delete failed")
	}
}

func TestTableDeleteNonExistsTo(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)
	to.MessengerID = "msgB"

	ret := <-tab.delete(from, to)
	if ret {
		t.Error("delete failed")
	}
}

func TestTableDeleteNonExistsFrom(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)
	from.MessengerID = "msgB"

	ret := <-tab.delete(from, to)
	if ret {
		t.Error("delete failed")
	}
}

func TestTableGetTo(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	toB := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chC"},
		Alias{SrcAlias: "src1", DstAlias: "dst2"},
	}

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		<-tab.insert(from, to)
		wg.Done()
	}()
	go func() {
		<-tab.insert(from, toB)
		wg.Done()
	}()
	wg.Wait()

	checkList := make(channelList)
	checkList[to.Channel] = to.Alias
	checkList[toB.Channel] = toB.Alias
	getList := tab.getTo(from)
	if !reflect.DeepEqual(checkList, getList) {
		t.Errorf("getTo failed: %s", getList)
	}
}

func TestTableInsertDuplicateAliasDstRename(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)

	to.Channel.ChannelID = "chC"
	to.Alias = Alias{SrcAlias: "src1", DstAlias: "dst"}

	ret := <-tab.insert(from, to)
	if !ret.ok {
		t.Error("insert duplicated dst failed")
	}

	if ret.SrcAlias != "src1" {
		t.Errorf("src alias is wrong")
	}

	if ret.DstAlias == "dst" {
		t.Errorf("dst alias is wrong")
	}
}

func TestTableInsertDuplicateAliasSrc(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)

	to.Channel.ChannelID = "chC"
	to.Alias = Alias{SrcAlias: "src", DstAlias: "dst1"}

	ret := <-tab.insert(from, to)
	if !ret.ok {
		t.Error("insert duplicated src failed")
	}

	if ret.SrcAlias != "src" {
		t.Errorf("src alias is wrong")
	}

	if ret.DstAlias != "dst1" {
		t.Errorf("dst alias is wrong")
	}
}

func TestTableInsertDuplicateAliasSrcRename(t *testing.T) {
	tab, cancel := getTestTable()
	defer cancel()
	from := telepathy.Channel{MessengerID: "msgA", ChannelID: "chA"}
	from2 := telepathy.Channel{MessengerID: "msgB", ChannelID: "chA"}
	to := TableEntry{
		telepathy.Channel{MessengerID: "msgA", ChannelID: "chB"},
		Alias{SrcAlias: "src", DstAlias: "dst"},
	}
	<-tab.insert(from, to)

	to.Alias = Alias{SrcAlias: "src", DstAlias: "dst"}

	ret := <-tab.insert(from2, to)
	if !ret.ok {
		t.Error("insert duplicated src failed")
	}

	if ret.SrcAlias == "src" {
		t.Errorf("src alias is wrong")
	}

	if ret.DstAlias != "dst" {
		t.Errorf("dst alias is wrong")
	}
}
