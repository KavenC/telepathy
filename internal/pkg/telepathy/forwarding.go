package telepathy

import "sync"

// Forwarding manages message forwardings between Channels
type Forwarding struct {
	sync.Mutex
	fwdTable map[Channel]map[Channel]bool
}

var fwd *Forwarding

func initForwarding() {
	// TODO: Read from database
	fwd = &Forwarding{
		fwdTable: make(map[Channel]map[Channel]bool),
	}
}

// GetForwarding gets a forwarding manager
func GetForwarding() *Forwarding {
	if fwd == nil {
		initForwarding()
	}
	return fwd
}

// CreateForwarding creates a new channel message forwarding
// if the forwarding already exsits, returns false
func (f *Forwarding) CreateForwarding(from Channel, to Channel) bool {
	f.Lock()
	defer f.Unlock()
	fromTbl, ok := f.fwdTable[from]
	if !ok {
		f.fwdTable[from] = make(map[Channel]bool)
	} else if fromTbl[to] {
		return false
	}

	f.fwdTable[from][to] = true
	// TODO: Write to DB
	return true
}

// GetForwardingTo gets a list of forwarding channels from the source Channel
func (f *Forwarding) GetForwardingTo(from Channel) map[Channel]bool {
	f.Lock()
	defer f.Unlock()
	fromTbl, ok := f.fwdTable[from]
	if !ok {
		return nil
	}
	return fromTbl
}

// GetForwardingFrom gets a list of channels that are forwarding to specified channel
func (f *Forwarding) GetForwardingFrom(to Channel) map[Channel]bool {
	from := make(map[Channel]bool)
	f.Lock()
	defer f.Unlock()
	for fromCh, toChList := range f.fwdTable {
		if toChList[to] {
			from[fromCh] = true
		}
	}

	if len(from) == 0 {
		return nil
	}
	return from
}

// RemoveForwarding remove a channel forwarding.
// If the forwarding does not exist, retruns false
func (f *Forwarding) RemoveForwarding(from Channel, to Channel) bool {
	f.Lock()
	defer f.Unlock()
	_, ok := f.fwdTable[from]
	if !ok {
		return false
	}

	_, ok = f.fwdTable[from][to]
	if ok {
		delete(f.fwdTable[from], to)
		// TODO: write to db
		return true
	}

	return false
}
