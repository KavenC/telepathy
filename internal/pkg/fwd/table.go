package fwd

import (
	"errors"
	"sync"

	"gitlab.com/kavenc/telepathy/internal/pkg/randstr"

	"github.com/mongodb/mongo-go-driver/bson"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	// Configurations
	nameRetry = 3
	queueLen  = 10
	// Action identifiers
	tableInsert = 0
	tableDelete = 1
	tableBSON   = 2
	tableDirty  = 3
)

// Alias is the alias information of a channel in forwarding table
type Alias struct {
	SrcAlias string
	DstAlias string
}

// The TableEntry in forwarding table
// This is public only to be serialized
type TableEntry struct {
	telepathy.Channel
	Alias
}

type channelList map[telepathy.Channel]Alias

type tableOp struct {
	action int
	key    telepathy.Channel
	entry  TableEntry
	ret    chan interface{}
}

type table struct {
	data    sync.Map
	opQueue chan tableOp
	dirty   bool
}

type insertRet struct {
	ok bool
	Alias
}

func newTable() *table {
	return &table{
		opQueue: make(chan tableOp, queueLen),
	}
}

func (ft *table) start() {
	for op := range ft.opQueue {
		if op.action == tableInsert {
			op.ret <- ft.insertImpl(op)
		} else if op.action == tableDelete {
			op.ret <- ft.deleteImpl(op)
		} else if op.action == tableBSON {
			op.ret <- ft.bsonImpl()
		} else if op.action == tableDirty {
			op.ret <- ft.dirtyImpl()
		}
	}
}

func (ft *table) stop() {
	close(ft.opQueue)
}

func (ft *table) insert(from telepathy.Channel, to TableEntry) chan insertRet {
	op := tableOp{
		action: tableInsert,
		key:    from,
		entry:  to,
		ret:    make(chan interface{}),
	}

	ft.opQueue <- op
	ret := make(chan insertRet, 1)
	go func() {
		opRet := <-op.ret
		iRet, _ := opRet.(insertRet)
		ret <- iRet
	}()

	return ret
}

func (ft *table) delete(from, to telepathy.Channel) chan bool {
	op := tableOp{
		action: tableDelete,
		key:    from,
		entry:  TableEntry{Channel: to},
		ret:    make(chan interface{}),
	}

	ft.opQueue <- op
	ret := make(chan bool, 1)
	go func() {
		opRet := <-op.ret
		iRet, _ := opRet.(bool)
		ret <- iRet
	}()

	return ret
}

func (ft *table) getFrom(to telepathy.Channel) channelList {
	ret := make(channelList)
	ft.data.Range(func(key, value interface{}) bool {
		from, _ := key.(telepathy.Channel)
		toList, _ := value.(channelList)
		if toEntry, ok := toList[to]; ok {
			ret[from] = toEntry
		}
		return true
	})
	return ret
}

func (ft *table) getTo(from telepathy.Channel) channelList {
	load, ok := ft.data.Load(from)
	if !ok {
		return nil
	}
	ret, _ := load.(channelList)
	return ret
}

func (ft *table) bson() chan *bson.A {
	op := tableOp{action: tableBSON, ret: make(chan interface{})}
	ft.opQueue <- op
	ret := make(chan *bson.A, 1)
	go func() {
		opRet := <-op.ret
		bs, _ := opRet.(*bson.A)
		ret <- bs
	}()
	return ret
}

func (ft *table) isDirty() chan bool {
	op := tableOp{action: tableDirty, ret: make(chan interface{})}
	ft.opQueue <- op
	ret := make(chan bool, 1)
	go func() {
		opRet := <-op.ret
		d, _ := opRet.(bool)
		ret <- d
	}()
	return ret
}

func (ft *table) insertImpl(op tableOp) insertRet {
	// Load list from sync map
	load, ok := ft.data.Load(op.key)
	var toList channelList
	if ok {
		toList, _ = load.(channelList)
	} else {
		toList = make(channelList)
	}

	// if from, to pair exists, return directly as false (not inserted)
	if _, ok = toList[op.entry.Channel]; ok {
		return insertRet{ok: false}
	}

	// Make sure src, dst alias are not duplicated and do insertion
	// ensure unique DstAlias
	pass := true
	for retry := nameRetry; retry > 0; retry-- {
		pass = true
		for _, to := range toList {
			if to.DstAlias == op.entry.DstAlias {
				op.entry.DstAlias += "_" + randstr.Generate(4)
				pass = false
				break
			}
		}

		if pass {
			break
		}
	}

	if !pass {
		return insertRet{ok: false}
	}

	// ensure unique SrcAlias
	fromList := ft.getFrom(op.entry.Channel)
	for retry := nameRetry; retry > 0; retry-- {
		pass = true
		for _, from := range fromList {
			if from.SrcAlias == op.entry.SrcAlias {
				op.entry.SrcAlias += "_" + randstr.Generate(4)
				pass = false
				break
			}
		}

		if pass {
			break
		}
	}
	if !pass {
		return insertRet{ok: false}
	}

	toList[op.entry.Channel] = op.entry.Alias
	ft.data.Store(op.key, toList)
	ft.dirty = true
	return insertRet{
		ok:    true,
		Alias: op.entry.Alias,
	}
}

func (ft *table) deleteImpl(op tableOp) bool {
	load, ok := ft.data.Load(op.key)
	if !ok {
		return false
	}
	toList, _ := load.(channelList)
	_, exists := toList[op.entry.Channel]
	if exists {
		delete(toList, op.entry.Channel)
		ft.data.Store(op.key, toList)
		ft.dirty = true
	}
	return exists
}

func (ft *table) bsonImpl() *bson.A {
	tableBSON := bson.A{}

	// Table to BSON
	ft.data.Range(func(key interface{}, value interface{}) bool {
		chListBSON := bson.A{}
		from, _ := key.(telepathy.Channel)
		toList, _ := value.(channelList)
		for toCh, alias := range toList {
			chListBSON = append(chListBSON, bson.A{toCh, alias})
		}
		tableBSON = append(tableBSON, bson.A{from, chListBSON})
		return true
	})

	ft.dirty = false

	return &tableBSON
}

func (ft *table) dirtyImpl() bool {
	return ft.dirty
}

// This function should only be used before start()
func (ft *table) loadBSON(input bson.RawValue) error {
	bsonArray, ok := input.ArrayOK()
	if !ok {
		return errors.New("bson.RawValue is not an array")
	}

	bsonElements, err := bsonArray.Elements()
	if err != nil {
		return err
	}

	for _, element := range bsonElements {
		pair := element.Value().Array()
		from := telepathy.Channel{}
		err = pair.Index(0).Value().Unmarshal(&from)
		if err != nil {
			return err
		}
		rawArray := pair.Index(1).Value().Array()
		rawList, err := rawArray.Elements()
		if err != nil {
			return err
		}
		list := make(channelList)
		for _, rawCh := range rawList {
			to := telepathy.Channel{}
			alias := Alias{}
			pair = rawCh.Value().Array()
			err = pair.Index(0).Value().Unmarshal(&to)
			if err != nil {
				return err
			}
			err = pair.Index(1).Value().Unmarshal(&alias)
			if err != nil {
				return err
			}
			list[to] = alias
		}
		ft.data.Store(from, list)
	}

	ft.dirty = false

	return nil
}
