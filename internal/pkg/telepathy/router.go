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
	receiverIn     map[string]<-chan InboundMessage
	receiverOut    map[string]chan InboundMessage
	transmitterIn  map[string]<-chan OutboundMessage
	transmitterOut map[string]chan OutboundMessage
	cmdOut         chan InboundMessage
	cmd            *cmdManager
	logger         *logrus.Entry
}

func newRouter() *router {
	rt := &router{
		receiverIn:     make(map[string]<-chan InboundMessage),
		receiverOut:    make(map[string]chan InboundMessage),
		transmitterIn:  make(map[string]<-chan OutboundMessage),
		transmitterOut: make(map[string]chan OutboundMessage),
		cmdOut:         make(chan InboundMessage, routerCmdLen),
		logger:         logrus.WithField("module", "router"),
	}
	cmd := newCmdManager(rt.cmdOut)
	rt.attachProducer("telepathy.cmd", cmd.msgOut)
	rt.cmd = cmd
	return rt
}

func (r *router) attachReceiver(id string, ch <-chan InboundMessage) {
	_, ok := r.receiverIn[id]
	if ok {
		r.logger.Panicf("receiver has already been attached: %s", id)
	}
	r.receiverIn[id] = ch
}

func (r *router) attachProducer(id string, ch <-chan OutboundMessage) {
	_, ok := r.transmitterIn[id]
	if ok {
		r.logger.Panicf("producer has already been attached: %s", id)
	}
	r.transmitterIn[id] = ch
}

func (r *router) attachConsumer(id string) <-chan InboundMessage {
	_, ok := r.receiverOut[id]
	if ok {
		r.logger.Panicf("consumer has already been attached: %s", id)
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

func (r *router) receiver() {
	logger := r.logger.WithField("phase", "receiver")
	logger.Info("started")

	// Collect Inbound Messages from all registered channels
	wg := sync.WaitGroup{}
	wg.Add(len(r.receiverIn))
	inMsgCh := make(chan InboundMessage, routerRecvHandleLen)
	forward := func(ch <-chan InboundMessage) {
		for msg := range ch {
			inMsgCh <- msg
		}
		wg.Done()
	}
	for _, inCh := range r.receiverIn {
		go forward(inCh)
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
			timeout, cancel := context.WithTimeout(context.Background(), routerRecvOutTimeout)
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
	for _, ch := range r.receiverOut {
		close(ch)
	}
	close(r.cmdOut)

	logger.Info("terminated")
}

func (r *router) transmitter() {
	logger := r.logger.WithField("phase", "transmitter")
	logger.Info("started")
	wg := sync.WaitGroup{}
	wg.Add(len(r.transmitterIn))
	outMsgCh := make(chan OutboundMessage, routerTranHandleLen)

	// Collect outbound messages
	forward := func(ch <-chan OutboundMessage) {
		for msg := range ch {
			outMsgCh <- msg
		}
		wg.Done()
	}
	for _, ch := range r.transmitterIn {
		go forward(ch)
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

		timeout, cancel := context.WithTimeout(context.Background(), routerTranOutTimeout)
		select {
		case ch <- msg:
		case <-timeout.Done():
			logger.Warnf("transmitter out timeout/cancelled: %s", id)
		}
		cancel()
	}

	for _, ch := range r.transmitterOut {
		close(ch)
	}
	logger.Info("terminated")
}

func (r *router) start() {
	wg := sync.WaitGroup{}
	wg.Add(3)
	go func() {
		r.cmd.start()
		wg.Done()
	}()
	go func() {
		r.receiver()
		wg.Done()
	}()
	go func() {
		r.transmitter()
		wg.Done()
	}()

	r.logger.Infof("started")
	wg.Wait()
	r.logger.Infof("terminated")
}
