package telepathy

import (
	"context"
	"fmt"
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

// CmdExtraArgs carries extra info for command handlers
type CmdExtraArgs struct {
	Ctx     context.Context
	Message InboundMessage
}

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
	done    chan interface{}
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
		done:   make(chan interface{}),
		logger: logrus.WithField("module", "cmdManager"),
	}
}

func (m *cmdManager) attachCommandInterface(cmd *argo.Action) {
	m.cmdRoot.AddSubAction(*cmd)
	m.logger.Infof("attached command: %s", cmd.Trigger)
}

func (m *cmdManager) isCmdMsg(text string) bool {
	return strings.HasPrefix(text, cmdPrefix+" ")
}

func (m *cmdManager) worker(id int) {
	logger := m.logger.WithField("worker", strconv.Itoa(id))

	// worker function for handling command messages
	for msg := range m.cmdIn {
		args := regexCmdSplitter.Split(msg.Text, -1)
		timeout, cancel := context.WithTimeout(context.Background(), cmdTimeout)
		done := make(chan interface{})

		go func() {
			state := argo.State{}
			err := m.cmdRoot.Parse(&state, args, CmdExtraArgs{
				Message: msg,
				Ctx:     timeout,
			})
			if err != nil {
				if _, ok := err.(argo.Err); ok {
					fmt.Fprintf(&state.OutputStr, "Invalid command: %s", err.Error())
				} else {
					logger.Errorf("command parsing failed: %s", err.Error())
					logger.Errorf("msg: %s", msg.Text)
					logger.Errorf("partial OutputStr: ")
					logger.Errorf(state.OutputStr.String())
					state.OutputStr.Reset()
					state.OutputStr.WriteString("Internal Error! Please try again later.")
				}
			}
			if state.OutputStr.Len() != 0 {
				msg := msg.Reply()
				msg.Text = state.OutputStr.String()
				m.msgOut <- msg
			}
			close(done)
		}()
		select {
		case <-done:
		case <-timeout.Done():
			logger.Warnf("timeout/cacnelled: %s", args)
		}
		cancel()
	}
}

func (m *cmdManager) start() {
	m.cmdRoot.Finalize()
	wg := sync.WaitGroup{}
	wg.Add(cmdWorkerNum)
	for id := 0; id < cmdWorkerNum; id++ {
		go func(id int) {
			m.worker(id)
			wg.Done()
		}(id)
	}

	m.logger.Infof("started worker: %d", cmdWorkerNum)
	wg.Wait()
	close(m.done)
	close(m.msgOut)
	m.logger.Info("termianted")
}

// CommandEnsureDM checks if command is from direct message
func CommandEnsureDM(state *argo.State, extraArgs CmdExtraArgs) bool {
	if !extraArgs.Message.IsDirectMessage {
		state.OutputStr.WriteString("This command can only be run with Direct Messages (Whispers).\n")
		return false
	}
	return true
}

// CommandPrefix returns command triggering keyword
func CommandPrefix() string {
	return cmdPrefix
}
