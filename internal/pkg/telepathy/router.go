package telepathy

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	routerRecvOutLen     = 1
	routerTranOutLen     = 1
	routerRecvHandleLen  = 1
	routerTranHandleLen  = 1
	routerCmdLen         = 10
	routerRecvOutTimeout = time.Second * 5
	routerTranOutTimeout = time.Second * 5
)

// Router consists of receiver and transmitter parts
// The terms: receiver and transmitter are in the view of telepathy server
// That is, receiver handles messages that are received from user input (through messengers)
// and transmitter handles messages to be sent to users through messengers
// The In/Out postfix indicates whether the message is comming in to or out from this router module
// Channels are usually connected as:
// +------------------+  -- receiverIn ------> +--------+ -- receiverOut ----> +----------------+
// | Messenger Plugin |                        | Router |                      | Service Plugin |
// +------------------+  <-- transmitterOut -- +--------+ <-- transmitterIn -- +----------------+
type router struct {
	receiverIn    map[string]<-chan InboundMessage
	receiverOut   map[string]chan InboundMessage
	transmitterIn map[string]<-chan OutboundMessage
	trasmitterOut map[string]chan OutboundMessage
	cmdOut        chan InboundMessage
	cmd           *cmdManager
	logger        *logrus.Entry
}

func newRouter() *router {
	rt = &router{
		receiverIn:     make(map[string]<-chan InboundMessage),
		receiverOut:    make(map[string]chan InboundMessage),
		transmitterIn:  make(map[string]<-chan OutboundMessage),
		transmitterOut: make(map[string]chan OutboundMessage),
		cmdOut:         make(chan InboundMessage, routerCmdLen),
		logger:         logrus.WithField("module", "router"),
	}
	cmd := newCmdManager(rt.cmdOut)
	rt.registerTransmitter("telepathy.cmd", cmd.msgOut)
	rt.cmd = cmd
	return rt
}

func (r *router) registerReceiver(id string, ch <-chan InboundMessage) {
	_, ok := r.receiverIn[id]
	if ok {
		r.logger.Panicf("receiver has already been registered: %s", id)
	}
	r.receiverIn[id] = ch
}

func (r *router) registerTransmitter(id string, ch <-chan OutboundMessage) {
	_, ok := r.transmitterIn[id]
	if ok {
		r.logger.Panicf("transmitter has already been registered: &s", id)
	}
	r.transmitterIn[id] = ch
}

func (r *router) attachReceiver(id string) <-chan InboundMessage {
	_, ok := r.receiverOut[id]
	if ok {
		r.logger.Panicf("receiver has already been attached: %s", id)
	}
	r.receiverOut[id] = make(chan InboundMessage, routerRecvOutLen)
	return r.receiverOut[id]
}

func (r *router) attachTransmitter(id string) <-chan OutboundMessage {
	_, ok := r.transmitterOut[id]
	if ok {
		r.logger.Panicf("transmitter has already been attached: %s", id)
	}
	r.transmitterOut[id] = make(chan OutboundMessage, routerTranOutLen)
	return r.transmitterOut[id]
}

func (r *router) receiver(ctx context.Context) {
	logger := r.logger.WithField("phase", "receiver")
	wg := sync.WaitGroup{}
	wg.Add(len(r.receiverIn))
	inMsgCh := make(chan InboundMessage, routerRecvHandleLen)

	// Collect Inbound Messages from all registered channels
	for id, inCh := range r.receiverIn {
		go func() {
			for msg := range inCh {
				inMsgCh <- msg
			}
			wg.Done()
		}()
	}

	// Close handling channel when all inbound channels are closed
	go func() {
		wg.Wait()
		close(inMsgCh)
	}()

	// Inbound Message handling
	for msg := range inMsgCh {
		// Pass to cmd manager if it is a command message
		if r.cmd.isCmdMsg(msg.Text) {
			r.cmdOut <- msg
			continue
		}

		// Forward message to all listeners
		for id, ch := range r.receiverOut {
			timeout, cancel := context.WithTimeout(ctx, routerRecvOutTimeout)
			select {
			case ch <- msg:
			case <-timeout.Done():
				logger.Warnf("receiver out timeout/cancelled on: %s", id)
			}
			cancel()
		}
	}

	// If reach here, the handling channel is emptied and closed
	// Close all outbound channels
	for id, ch := range r.receiverOut {
		close(ch)
	}
}

func (r *router) transmitter(ctx context.Context) {
	logger := r.logger.WithField("phase", "transmitter")
	wg := sync.WaitGroup{}
	wg.Add(len(r.transmitterIn))
	outMsgCh := make(chan OutboundMessage, routerTranHandleLen)

	// Collect outbound messages
	for id, ch := range r.transmitterIn {
		go func() {
			for msg := range ch {
				outMsgCh <- msg
			}
			wg.Done()
		}()
	}

	// Close handling channel when all outbound message channels are closed
	go func() {
		wg.Wait()
		close(outMsgCh)
	}()

	// Outbound message handling
	for msg := range outMsgCh {
		id := msg.ToChannel.MessengerID
		ch, ok := r.transmitterOut[id]
		if !ok {
			logger.Errorf("messenger not found: %s", id)
			continue
		}

		timeout, cancel := context.WithTimeout(ctx, routerTranOutTimeout)
		select {
		case ch <- msg:
		case timeout.Done():
			logger.Warnf("transmitter out timeout/cancelled: %s", id)
		}
		cancel()
	}

	for id, ch := range r.transmitterOut {
		close(ch)
	}
}

func (r *router) start(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		r.receiver(ctx)
		wg.Done()
	}()
	go func() {
		r.transmitter(ctx)
		wg.Done()
	}()

	r.logger.Infof("started")
	wg.Wait()
	r.logger.Infof("terminated")
}
