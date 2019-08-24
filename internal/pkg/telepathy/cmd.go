package telepathy

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.com/kavenc/argo"
)

const (
	cmdMsgOutLen = 5
)

// CmdExtraArgs carries extra info for command handlers
type CmdExtraArgs struct {
	Prefix  string
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
	cmdRoot   argo.Action
	cmdIn     <-chan InboundMessage
	msgOut    chan OutboundMessage
	workerNum uint
	timeout   time.Duration
	done      chan interface{}
	logger    *logrus.Entry
}

var regexCmdSplitter = regexp.MustCompile(" +")

func newCmdManager(prefix string, workerNum uint, timeout time.Duration, cmdMsgCh <-chan InboundMessage) *cmdManager {
	return &cmdManager{
		cmdRoot: argo.Action{
			Trigger: prefix,
		},
		cmdIn:     cmdMsgCh,
		msgOut:    make(chan OutboundMessage, cmdMsgOutLen),
		workerNum: workerNum,
		timeout:   timeout,
		done:      make(chan interface{}),
		logger:    logrus.WithField("module", "cmdManager"),
	}
}

func (m *cmdManager) attachCommandInterface(cmd *argo.Action) error {
	err := m.cmdRoot.AddSubAction(*cmd)
	if err != nil {
		return err
	}
	m.logger.Infof("attached command: %s", cmd.Trigger)
	return nil
}

func (m *cmdManager) isCmdMsg(text string) bool {
	return strings.HasPrefix(text, m.cmdRoot.Trigger+" ")
}

func (m *cmdManager) worker(ctx context.Context, id uint) {
	logger := m.logger.WithField("worker", fmt.Sprint(id))

	// worker function for handling command messages
	for msg := range m.cmdIn {
		args := regexCmdSplitter.Split(msg.Text, -1)
		timeout, cancel := context.WithTimeout(ctx, m.timeout)
		done := make(chan interface{})

		go func() {
			state := argo.State{}
			err := m.cmdRoot.Parse(&state, args, CmdExtraArgs{
				Prefix:  m.cmdRoot.Trigger,
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

func (m *cmdManager) start(ctx context.Context) {
	m.cmdRoot.Finalize()
	wg := sync.WaitGroup{}
	wg.Add(int(m.workerNum))
	for id := uint(0); id < m.workerNum; id++ {
		go func(id uint) {
			m.worker(ctx, id)
			wg.Done()
		}(id)
	}

	m.logger.Infof("started worker: %d", m.workerNum)
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
