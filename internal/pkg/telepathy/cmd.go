package telepathy

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
)

const (
	cmdPrefix    = "teru"
	cmdWorkerNum = 10
	cmdMsgOutLen = 5
	cmdTimeout   = time.Second * 2
)

type commandMessage struct {
	msg  InboundMessage
	args []string
}

type commandHandler struct {
	cmd     *argo.Action
	channel chan commandMessage
}

type cmdManager struct {
	cmdRoot argo.Action
	cmdIn   <-chan InboundMessage
	msgOut  chan OutboundMessage
	logger  *logrus.Entry
}

var regexCmdSplitter = regexp.MustCompile(" +")

func newCmdManager(inCh <-chan InboundMessage) *cmdManager {
	return &cmdManager{
		cmdRoot: argo.Action{
			Trigger: cmdPrefix,
		},
		cmdIn:  inCh,
		msgOut: make(chan OutboundMessage, cmdMsgOutLen),
		logger: logrus.WithField("module", "cmdManager"),
	}
}

func (m *cmdManager) AttachCommandInterface(cmd *argo.Action) {
	m.cmdRoot.AddSubAction(*cmd)
	logger.Infof("attached command: %s", cmd.Trigger)
}

func (m *cmdManager) isCmdMsg(text string) bool {
	return strings.HasPrefix(text, cmdPrefix+" ")
}

func (m *cmdManager) worker(ctx context.Context, id int) {
	logger := m.logger.WithField("worker", strconv.Itoa(id))

	// worker function for handling command messages
	for msg := range m.cmdIn {
		args := regexCmdSplitter.Split(msg.Text, -1)
		timeout, cancel := context.WithTimeout(ctx, cmdTimeout)
		state := argo.State{}
		select {
		case m.cmdRoot.Parse(&state, args, timeout):
			if state.OutputStr.Len() != 0 {
				msg := msg.Reply()
				msg.Text = state.OutputStr.String()
				m.msgOut <- msg
			}
		case timeout.Done():
			logger.Warnf("timeout/cacnelled: %s", args)
		}
		cancel()
	}
}

func (m *cmdManager) start(ctx context.Context) {
	wg := sync.WaitGroup{}
	wg.Add(cmdWorkerNum)
	for id := 0; id < cmdWorkerNum; id++ {
		go func() {
			m.worker(ctx, id)
			wg.Done()
		}()
	}

	m.logger.Info("started")
	wg.Wait()
	close(m.msgOut)
	m.logger.Info("termianted")
}
