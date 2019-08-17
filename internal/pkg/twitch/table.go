package twitch

import (
	"sync"

	"github.com/mongodb/mongo-go-driver/bson"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

type table struct {
	lock  sync.RWMutex
	data  map[string]map[telepathy.Channel]bool
	dirty bool
}

func newTable() *table {
	return &table{
		data: make(map[string]map[telepathy.Channel]bool),
	}
}

func (t *table) isDirty() bool {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.dirty
}

func (t *table) getKeys() []string {
	t.lock.RLock()
	defer t.lock.RUnlock()
	ret := make([]string, 0, len(t.data))
	for id := range t.data {
		ret = append(ret, id)
	}
	return ret
}

func (t *table) lookUpOrAdd(key string, channel telepathy.Channel) (keyExists bool, chExists bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	chmap, keyExists := t.data[key]
	if !keyExists {
		chExists = false
		t.data[key] = map[telepathy.Channel]bool{channel: true}
		t.dirty = true
		return
	}
	_, chExists = chmap[channel]
	if !chExists {
		chmap[channel] = true
		t.dirty = true
	}
	return
}

func (t *table) contains(channel telepathy.Channel) []string {
	t.lock.RLock()
	defer t.lock.RUnlock()
	ret := make([]string, 0, len(t.data))
	for key, list := range t.data {
		if _, ok := list[channel]; ok {
			ret = append(ret, key)
		}
	}
	return ret
}

func (t *table) remove(key string, channel telepathy.Channel) (keyRemoved bool, chRemoved bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	chmap, keyExists := t.data[key]
	if !keyExists {
		return false, false
	}
	_, chExists := chmap[channel]
	if !chExists {
		return false, false
	}
	t.dirty = true
	delete(chmap, channel)
	if len(chmap) == 0 {
		delete(t.data, key)
		return true, true
	}
	return false, true
}

// add an item into table.
// Returns True if the item does not exist and added.
//         False if the item already exists
func (t *table) add(key string, channel telepathy.Channel) bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	chmap, ok := t.data[key]
	if !ok {
		t.data[key] = map[telepathy.Channel]bool{channel: true}
		t.dirty = true
		return true
	}

	if chmap[channel] {
		return false
	}

	chmap[channel] = true
	t.dirty = true
	return true
}

func (t *table) hasKey(key string) bool {
	t.lock.RLock()
	defer t.lock.RUnlock()
	_, ok := t.data[key]
	return ok
}

func (t *table) getList(key string) (map[telepathy.Channel]bool, bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()
	copy := make(map[telepathy.Channel]bool)
	list, ok := t.data[key]
	if !ok {
		return copy, ok
	}
	for ch := range list {
		copy[ch] = true
	}
	return copy, ok
}

func (t *table) bson() *bson.A {
	t.lock.Lock()
	defer t.lock.Unlock()
	array := bson.A{}

	for key, list := range t.data {
		chArray := bson.A{}
		for channel := range list {
			chArray = append(chArray, channel)
		}
		array = append(array, bson.A{key, chArray})
	}

	t.dirty = false
	return &array
}

func (t *table) fromBSON(raw bson.RawValue) error {
	t.lock.Lock()
	t.lock.Unlock()
	array := raw.Array()

	elements, err := array.Elements()
	if err != nil {
		return err
	}

	t.data = make(map[string]map[telepathy.Channel]bool)
	for _, element := range elements {
		pair := element.Value().Array()
		var key string
		err = pair.Index(0).Value().Unmarshal(&key)
		if err != nil {
			return err
		}

		chRawArray := pair.Index(1).Value().Array()
		chElements, err := chRawArray.Elements()
		if err != nil {
			return err
		}

		list := make(map[telepathy.Channel]bool)
		for _, rawCh := range chElements {
			var ch telepathy.Channel
			err = rawCh.Value().Unmarshal(&ch)
			if err != nil {
				return err
			}
			list[ch] = true
		}

		t.data[key] = list
	}

	t.dirty = false
	return nil
}
