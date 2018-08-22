package fwd

import (
	"context"
	"fmt"
	"sync"

	"gitlab.com/kavenc/telepathy/internal/pkg/telepathy"
)

type channelList map[telepathy.Channel]bool

type forwardingManager struct {
	sync.Mutex
	table *sync.Map
}

func init() {
	telepathy.RegisterMessageHandler(msgHandler)
}

func manager(s *telepathy.Session) *forwardingManager {
	m, _ := s.Resrc.LoadOrStore(funcKey, &forwardingManager{table: &sync.Map{}})
	ret, ok := m.(*forwardingManager)
	if !ok {
		logger.Error("Unable to get resource: forwardingManager")
		// Return a dummy manager to keeps things going
		// But it wont work well for sure
		return &forwardingManager{table: &sync.Map{}}
	}
	return ret
}

func (m *forwardingManager) createForwarding(from, to telepathy.Channel) bool {
	newTo := channelList{to: true}
	m.Lock()
	defer m.Unlock()
	load, ok := m.table.LoadOrStore(from, newTo)
	if ok {
		existsToList, _ := load.(channelList)
		if existsToList[to] {
			return false
		}
		existsToList[to] = true
		m.table.Store(from, existsToList)
		return true
	}
	return true
}

func (m *forwardingManager) removeForwarding(from, to telepathy.Channel) bool {
	m.Lock()
	defer m.Unlock()
	load, ok := m.table.Load(from)
	if !ok {
		return false
	}
	toList, _ := load.(channelList)
	if toList[to] {
		delete(toList, to)
		m.table.Store(from, toList)
		return true
	}
	return false
}

func (m *forwardingManager) forwardingTo(from telepathy.Channel) channelList {
	load, ok := m.table.Load(from)
	if !ok {
		return nil
	}
	ret, _ := load.(channelList)
	return ret
}

func (m *forwardingManager) forwardingFrom(to telepathy.Channel) channelList {
	ret := make(channelList)
	m.table.Range(func(key, value interface{}) bool {
		toList, _ := value.(channelList)
		if toList[to] {
			from, _ := key.(telepathy.Channel)
			ret[from] = true
		}
		return true
	})

	if len(ret) == 0 {
		return nil
	}
	return ret
}

func msgHandler(ctx context.Context, t *telepathy.Session, message telepathy.InboundMessage) {
	manager := manager(t)
	toChList := manager.forwardingTo(message.FromChannel)
	if toChList != nil {
		text := fmt.Sprintf("[%s] %s:\n%s",
			message.FromChannel.MessengerID,
			message.SourceProfile.DisplayName,
			message.Text)
		for toCh := range toChList {
			outMsg := &telepathy.OutboundMessage{
				TargetID: toCh.ChannelID,
				Text:     text,
			}
			msgr, _ := t.Msgr.Messenger(toCh.MessengerID)
			msgr.Send(outMsg)
		}
	}
}
