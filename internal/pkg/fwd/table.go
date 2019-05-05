package fwd

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

const (
	// Configurations
	nameRetry   = 3
	queueLen    = 10
	// Action identifiers
	tableInsert = 0
	tableDelete = 1
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
	logger  *logrus.Entry
}

type insertRet struct {
	ok bool
	Alias
}

func (ft *table) start(ctx context.Context) {
	if ft.opQueue == nil {
		ft.opQueue = make(chan tableOp, queueLen)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				ft.logger.Info("terminated")
				return
			case op := <-ft.opQueue:
				if op.action == tableInsert {
					op.ret <- ft.insertImpl(op)
				} else if op.action == tableDelete {
					op.ret <- ft.deleteImpl(op)
				}
			}
		}
	}()
}

func (ft *table) insert(from telepathy.Channel, to TableEntry) chan insertRet {
	op := tableOp{
		action: tableInsert,
		key:    from,
		entry:  to,
		ret:    make(chan interface{}),
	}

	ft.opQueue <- op
	ret := make(chan insertRet)
	go func() {
		opRet := <-op.ret
		iRet, _ := opRet.(insertRet)
		ret <- iRet
	}()

	return ret
}

func (ft *table) delete(from telepathy.Channel, to TableEntry) chan bool {
	op := tableOp{
		action: tableDelete,
		key:    from,
		entry:  to,
		ret:    make(chan interface{}),
	}

	ft.opQueue <- op
	ret := make(chan bool)
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
				op.entry.DstAlias += "_" + telepathy.RandStr(4)
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
				op.entry.SrcAlias += "_" + telepathy.RandStr(4)
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
	}
	return exists
}
