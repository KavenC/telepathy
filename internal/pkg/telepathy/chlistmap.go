package telepathy

import (
	"context"
	"reflect"
	"sync"

	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/options"

	"github.com/mongodb/mongo-go-driver/bson"
)

// ChannelList is a map of channels
type ChannelList map[Channel]bool

// ChannelListMap is a thread safe data structure for storing multiple {key -> [channel, channel, ...]} entries
// This data structure is commonly used to manage channel data for services
type ChannelListMap struct {
	rwlock sync.RWMutex
	table  map[interface{}]ChannelList
}

// NewChannelListMap constructs and returns a pointer of ChannelListMap
func NewChannelListMap() *ChannelListMap {
	return &ChannelListMap{table: make(map[interface{}]ChannelList)}
}

func (c *ChannelListMap) bson() *bson.A {
	result := bson.A{}
	for key, chList := range c.table {
		ba := bson.A{}
		for ch := range chList {
			ba = append(ba, ch)
		}
		result = append(result, bson.A{key, ba})
	}
	return &result
}

// AddChannel append channel to list with key. Returns false if channel is already in the list, otherwise returns true
func (c *ChannelListMap) AddChannel(key interface{}, channel Channel) bool {
	c.rwlock.Lock()
	defer c.rwlock.Unlock()
	if _, ok := c.table[key]; ok {
		if _, ok = c.table[key][channel]; ok {
			return false
		}
		c.table[key][channel] = true
		return true
	}
	c.table[key] = ChannelList{channel: true}
	return true
}

// DelChannel deletes channel from list with key. Returns true if the channel existed and deleted, false if the channel does not exist
func (c *ChannelListMap) DelChannel(key interface{}, channel Channel) bool {
	c.rwlock.Lock()
	defer c.rwlock.Unlock()
	if chList, ok := c.table[key]; ok {
		if chList[channel] {
			delete(chList, channel)
			if len(chList) == 0 {
				delete(c.table, key)
			}
			return true
		}
	}
	return false
}

// GetList returns the ChannelList with specified key. The exists result is true if the key exists, otherwise false.
// If the exists result is false, returned chList must be ignored
func (c *ChannelListMap) GetList(key interface{}) (chList ChannelList, exists bool) {
	c.rwlock.RLock()
	defer c.rwlock.RUnlock()
	list, exists := c.table[key]
	if exists {
		chList = make(ChannelList)
		for k := range list {
			chList[k] = true
		}
	}
	return
}

func (c *ChannelListMap) setList(key interface{}, chList ChannelList) {
	c.table[key] = chList
}

// Range calls f sequentially for each key and value present in the map. If f returns false, range stops the iteration.
// Range always corresponds a snapshot of ChannelListMap.
func (c *ChannelListMap) Range(f func(key interface{}, chList ChannelList) bool) {
	c.rwlock.Lock()
	defer c.rwlock.Unlock()
	for key, chList := range c.table {
		if !f(key, chList) {
			return
		}
	}
}

// KeyExists checks whether ChannelListMap contains provided key
func (c *ChannelListMap) KeyExists(key interface{}) bool {
	c.rwlock.RLock()
	defer c.rwlock.RUnlock()
	_, ok := c.table[key]
	return ok
}

// LoadFromDB fills ChannelListMap with content from database
func (c *ChannelListMap) LoadFromDB(ctx context.Context, collection *mongo.Collection,
	id string, keyType reflect.Type) error {
	result := collection.FindOne(ctx, map[string]string{"ID": id})
	raw, err := result.DecodeBytes()
	if err != nil {
		return err
	}

	clmRaw := raw.Lookup("CLM")
	clmArray := clmRaw.Array()
	clmElems, err := clmArray.Elements()
	if err != nil {
		return err
	}

	c.rwlock.Lock()
	defer c.rwlock.Unlock()
	for _, elm := range clmElems {
		elmV := elm.Value()
		pair := elmV.Array()
		keyObjPtr := reflect.New(keyType)
		err = pair.Index(0).Value().Unmarshal(keyObjPtr.Interface())
		if err != nil {
			return err
		}
		rawChArray := pair.Index(1).Value().Array()
		rawChList, err := rawChArray.Elements()
		if err != nil {
			return err
		}
		chList := make(ChannelList)
		for _, rawCh := range rawChList {
			ch := Channel{}
			err = rawCh.Value().Unmarshal(&ch)
			if err != nil {
				return err
			}
			chList[ch] = true
		}
		c.setList(keyObjPtr.Elem().Interface(), chList)
	}

	return nil
}

// StoreToDB store ChannelListMap to mongo database
func (c *ChannelListMap) StoreToDB(ctx context.Context, collection *mongo.Collection,
	id string) (*mongo.UpdateResult, error) {
	c.rwlock.RLock()
	defer c.rwlock.RUnlock()
	bsonStruct := bson.M{
		"ID":  id,
		"CLM": *c.bson(),
	}
	return collection.ReplaceOne(ctx, map[string]string{"ID": id}, bsonStruct, options.Replace().SetUpsert(true))
}
