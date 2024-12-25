package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"sync"
)

type ManagerConfigOpts func(config *ManagerConfig)

type Manager struct {
	*managerHelper

	token   string
	id      string
	address string

	ctx      context.Context
	cancel   context.CancelFunc
	mux      *sync.Mutex
	reader   []*AgentReader[*Request]
	writer   *AgentWriter
	message  chan *Request
	response chan *TopicResponse
	config   *ManagerConfig
	wg       *sync.WaitGroup
	agent    *Agent
}

func (m *Manager) defaultConfig() {
	m.config = &ManagerConfig{
		OnConnectAfterFunc: func(requestId, msg string) {
			register := m.newRegister(m.token, m.id)
			m.writerResponse(register)
			logMsg := m.newLogMsg(m.token, m.id, []byte(msg))
			m.writerResponse(logMsg)
		},
		OnAgentErrorFunc: func(requestId string, err error) {
			msg := m.newLogMsg(m.token, m.id, []byte(fmt.Sprintf("agent error: %s", err)))
			m.writerResponse(msg)
		},
		debug: false,
		KafkaConfig: &KafkaConfig{
			timeout:  3,
			maxBytes: 1024 * 1024 * 10,
			retry:    3,
		},
		AgentConfig: &AgentConfig{
			TaskConfig: &TaskConfig{
				OnTaskStartFunc: func(requestId, taskId string, message TaskRequestMessage) {
					msg := m.newLogMsg(m.token, m.id, []byte(fmt.Sprintf("task: %s prepare running. params is: %s", taskId, string(message.Params))))
					m.writerResponse(msg)
				},
				OnTaskResultBackFunc: func(requestId, taskId string, message any) {
					response := m.newTaskResponse(m.token, m.id, message)
					m.writerResponse(response)
				},
				OnTaskFinishFunc: func(taskId string) {
					//process := m.newTaskProcess(m.token, m.id, taskId, 100)
					//m.writerResponse(process)
					msg := m.newLogMsg(m.token, m.id, []byte(fmt.Sprintf("task: %s is finish", taskId)))
					m.writerResponse(msg)
				},
				OnTaskStopFunc: func(requestId, taskId string) {
					msg := m.newLogMsg(m.token, m.id, []byte(fmt.Sprintf("task: %s is stop", taskId)))
					m.writerResponse(msg)
				},
				TaskProcess: func(taskId string, msg []byte) {
					process := m.newTaskProcess(m.token, m.id, taskId, 100)
					m.writerResponse(process)
				},
			},
			OnHealthFunc: func(msg []byte) {
				hearth := m.newHearth(m.token, m.id, msg)
				m.writerResponse(hearth)
			},
		},
	}
}
func (m *Manager) Start(ctx context.Context) error {
	childCtx, cancelFunc := context.WithCancel(ctx)
	m.ctx = childCtx
	m.cancel = cancelFunc
	m.reader = append(m.reader, NewReader[*Request](m.ctx, m.address, fmt.Sprintf("palm-%s", uuid.NewString()), ManagerTopic, m.config.KafkaConfig))
	m.reader = append(m.reader, NewReader[*Request](m.ctx, m.address, "palm-task", TaskTopic, m.config.KafkaConfig))
	m.writer = NewWriter(m.ctx, m.address, m.config.KafkaConfig)
	go func() {
		for _, reader := range m.reader {
			_reader := reader
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				for {
					select {
					case <-m.ctx.Done():
						return
					case m.message <- <-_reader.ReadMessage(m.ctx):
					}
				}
			}()
		}
		m.wg.Wait()
		close(m.message)
	}()
	err := m.agent.Start(m.ctx)
	if err != nil {
		m.config.OnAgentErrorFunc("", err)
		return err
	}
	m.config.OnConnectAfterFunc("", "agent connect success")
	go func() {
		for response := range m.response {
			select {
			case <-m.ctx.Done():
				return
			default:
				err2 := m.writer.writeMessage(response.Response, response.Topic)
				if err2 != nil {
					log.Errorf("write message fail: %s", err2)
					return
				}
			}
		}
	}()
	m.service()
	return nil
}
func (m *Manager) service() {
	for request := range m.message {
		log.Infof("process message id: %s,content: %s", string(request.RequestId), string(request.Msg))
		select {
		case <-m.ctx.Done():
			return
		default:
			if request.Token != "" {
				if m.token != request.Token {
					continue
				}
			}
			m.processMessage(request)
		}
	}
}

func (m *Manager) processMessage(request *Request) {
	if request.Token != "" && request.Token != m.token {
		return
	}
	writeError := func(err error) {
		if m.config.OnAgentErrorFunc != nil {
			m.config.OnAgentErrorFunc(request.RequestId, err)
		}
		return
	}
	switch request.Message.Type {
	case TaskRequest:
		var req = TaskRequestMessage{}
		if err := json.Unmarshal(request.Msg, &req); err != nil {
			writeError(err)
			return
		}
		m.agent.AddTask(&req)
	case ManagerRequest:
		var req = ManagerMsg{}
		if err := json.Unmarshal(request.Msg, &req); err != nil {
			writeError(err)
			return
		}
		m.processManagerMessage(request.RequestId, &req)
	default:
	}
}
func (m *Manager) writerResponse(response *TopicResponse) {
	go func() {
		select {
		case <-m.ctx.Done():
		case m.response <- response:
		}
	}()
}
func (m *Manager) processManagerMessage(rid string, request *ManagerMsg) {
	switch request.Typ {
	case StartTask:
		m.agent.starkTask(request.TaskId)
	case StopTask:
		m.agent.StopTask(request.TaskId)
	case ReuseTask:
	case ShutDownAgent:
		m.agent.shutDown()
	case RestartAgent:
		err := m.agent.Start(m.ctx)
		if err != nil {
			m.config.OnAgentErrorFunc(rid, err)
		}
	default:
		m.config.OnAgentErrorFunc(rid, fmt.Errorf("no process this manager type"))
	}
}
func NewManager(id string, address string, opts ...ManagerConfigOpts) *Manager {
	m := new(Manager)
	m.defaultConfig()
	for _, opt := range opts {
		opt(m.config)
	}
	m.token = uuid.NewString()
	m.id = id
	m.address = address
	m.mux = &sync.Mutex{}
	m.agent = NewAgent(m.config.AgentConfig)
	m.message = make(chan *Request, 1024)
	m.response = make(chan *TopicResponse, 1024)
	m.wg = &sync.WaitGroup{}
	m.managerHelper = new(managerHelper)
	return m
}

func (m *Manager) Finish() {
	m.cancel()
}
