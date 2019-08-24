package telepathy

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestRouterRecv(t *testing.T) {
	assert := assert.New(t)
	router := newRouter()
	recvr := make(chan InboundMessage)
	router.attachReceiver("recvr", recvr)
	consumerA := router.attachConsumer("A")
	consumerB := router.attachConsumer("B")
	done := make(chan interface{})
	go func() {
		router.start(context.Background(), time.Second, time.Second)
		close(done)
	}()

	msg := InboundMessage{
		FromChannel: Channel{
			MessengerID: "msg",
			ChannelID:   "ch",
		},
		Text: "test",
	}

	recvr <- msg
	close(recvr)
	<-done

	assert.Equal(1, len(consumerA))
	assert.Equal(1, len(consumerB))

	assert.Equal(msg, <-consumerA)
	assert.Equal(msg, <-consumerB)
}

func TestRouterRecvTimeout(t *testing.T) {
	assert := assert.New(t)
	router := newRouter()
	recvr := make(chan InboundMessage)
	router.attachReceiver("recvr", recvr)
	router.attachConsumer("TimeoutConsumer")

	done := make(chan interface{})
	go func() {
		router.start(context.Background(), time.Second, time.Second)
		close(done)
	}()

	msg := InboundMessage{
		FromChannel: Channel{
			MessengerID: "msg",
			ChannelID:   "ch",
		},
		Text: "test",
	}

	var log bytes.Buffer
	logrus.SetOutput(&log)
	defer logrus.SetOutput(os.Stderr)

	for {
		start := time.Now()
		recvr <- msg
		end := time.Now()
		if end.Sub(start) >= time.Second {
			assert.WithinDuration(start, end, 1200*time.Millisecond)
			break
		}
	}
	close(recvr)
	<-done
	assert.Contains(log.String(), "timeout")
	assert.Contains(log.String(), "TimeoutConsumer")
}

func TestRouterTrans(t *testing.T) {
	assert := assert.New(t)
	router := newRouter()
	prod := make(chan OutboundMessage)
	router.attachProducer("prod", prod)
	transA := router.attachTransmitter("transA")
	transB := router.attachTransmitter("transB")

	done := make(chan interface{})
	go func() {
		router.start(context.Background(), time.Second, time.Second)
		close(done)
	}()

	msgA := OutboundMessage{
		ToChannel: Channel{
			MessengerID: "transA",
		},
		Text: "smth",
	}
	msgB := OutboundMessage{
		ToChannel: Channel{
			MessengerID: "transB",
		},
		Text: "smth else",
	}

	prod <- msgA
	prod <- msgB

	close(prod)
	<-done

	assert.Equal(1, len(transA))
	assert.Equal(1, len(transB))
	assert.Equal(msgA, <-transA)
	assert.Equal(msgB, <-transB)
}

func TestRouterTransTimeout(t *testing.T) {
	assert := assert.New(t)
	router := newRouter()
	prod := make(chan OutboundMessage)
	router.attachProducer("prod", prod)
	router.attachTransmitter("TimeoutTransmitter")

	done := make(chan interface{})
	go func() {
		router.start(context.Background(), time.Second, time.Second)
		close(done)
	}()

	msgA := OutboundMessage{
		ToChannel: Channel{
			MessengerID: "TimeoutTransmitter",
		},
		Text: "smth",
	}

	var log bytes.Buffer
	logrus.SetOutput(&log)
	defer logrus.SetOutput(os.Stderr)

	for {
		start := time.Now()
		prod <- msgA
		end := time.Now()
		if end.Sub(start) >= time.Second {
			assert.WithinDuration(start, end, 1200*time.Millisecond)
			break
		}
	}

	close(prod)
	<-done

	assert.Contains(log.String(), "timeout")
	assert.Contains(log.String(), "TimeoutTransmitter")
}

func TestRouterAttachDuplicated(t *testing.T) {
	assert := assert.New(t)
	router := newRouter()

	router.attachConsumer("cons")
	assert.Panics(func() { router.attachConsumer("cons") })
	router.attachTransmitter("trans")
	assert.Panics(func() { router.attachTransmitter("trans") })

	recv := make(chan InboundMessage)
	router.attachReceiver("recv", recv)
	assert.Panics(func() { router.attachReceiver("recv", recv) })
	prod := make(chan OutboundMessage)
	router.attachProducer("prod", prod)
	assert.Panics(func() { router.attachProducer("prod", prod) })
}
