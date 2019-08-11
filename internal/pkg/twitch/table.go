package twitch

import (
	"sync"

	"github.com/mongodb/mongo-go-driver/bson"
	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

type table struct {
	lock sync.RWMutex
	data map[string]map[telepathy.Channel]bool
}

func newTable() *table {
	return &table{
		data: make(map[string]map[telepathy.Channel]bool),
	}
}

func (t *table) lookUpOrAdd(key string, channel telepathy.Channel) (keyExists bool, chExists bool) {
	t.lock.Lock()
	defer t.lock.Unlock()
	chmap, keyExists := t.data[key]
	if !keyExists {
		chExists = false
		t.data[key] = map[telepathy.Channel]bool{channel: true}
		return
	}
	_, chExists = chmap[channel]
	if !chExists {
		chmap[channel] = true
	}
	return
}

func (t *table) remove(key string, channel telepathy.Channel) bool {
	t.lock.Lock()
	defer t.lock.Unlock()
	chmap, keyExists := t.data[key]
	if !keyExists {
		return false
	}
	_, chExists := chmap[channel]
	if !chExists {
		return false
	}
	delete(chmap, channel)
	if len(chmap) == 0 {
		delete(t.data, key)
	}
	return true
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
		return true
	}

	if chmap[channel] {
		return false
	}

	chmap[channel] = true
	return true
}

func (t *table) getList(key string) (list map[telepathy.Channel]bool, ok bool) {
	t.lock.RLock()
	defer t.lock.RUnlock()
	list, ok = t.data[key]
	return
}

func (t *table) bson() *bson.A {
	t.lock.RLock()
	defer t.lock.RUnlock()
	array := bson.A{}

	for key, list := range t.data {
		chArray := bson.A{}
		for channel := range list {
			chArray = append(chArray, channel)
		}
		array = append(array, bson.A{key, chArray})
	}

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

	return nil
}
