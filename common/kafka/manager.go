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
		OnConnectBeforeFunc: func(requestId, msg string) {
			go func() {
				m.response <- &TopicResponse{
					Topic:    Log,
					Response: NewResponse(AgentLog, m.id, requestId, m.token, []byte(msg)),
				}
			}()
		},
		OnConnectAfterFunc: func(requestId, msg string) {
			go func() {
				m.response <- &TopicResponse{
					Topic:    CallBack,
					Response: NewResponse(Register, m.id, "", m.token, nil),
				}
				m.response <- &TopicResponse{
					Topic:    Log,
					Response: NewResponse(AgentLog, m.id, requestId, m.token, []byte(msg)),
				}
			}()
		},
		OnAgentErrorFunc: func(requestId string, err error) {
			go func() {
				m.response <- &TopicResponse{
					Topic:    Log,
					Response: NewResponse(AgentLog, m.id, requestId, m.token, []byte(fmt.Sprintf("agent start fail：%s", err))),
				}
			}()
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
					go func() {
						m.response <- &TopicResponse{
							Topic:    Log,
							Response: NewResponse(AgentLog, m.id, requestId, m.token, []byte(fmt.Sprintf("task: %s is prepare running.params is: %s", taskId, string(message.Params)))),
						}
					}()
				},
				OnTaskResultBackFunc: func(requestId, taskId string, message []byte) {
					go func() {
						m.response <- &TopicResponse{
							Topic:    CallBack,
							Response: NewResponse(TaskResponse, m.id, requestId, m.token, message),
						}
					}()
				},
				OnTaskFinishFunc: func(taskId string) {
					go func() {
						m.response <- &TopicResponse{
							Topic:    Log,
							Response: NewResponse(AgentLog, m.id, "", m.token, []byte(fmt.Sprintf("task finish: %s", taskId))),
						}
					}()
				},
				OnTaskStopFunc: func(requestId, taskId string) {
					go func() {
						m.response <- &TopicResponse{
							Topic:    Log,
							Response: NewResponse(AgentLog, m.id, requestId, m.token, []byte(fmt.Sprintf("task: %s is stop", taskId))),
						}
					}()
				},
				TaskProcess: func(taskId string, msg []byte) {
					go func() {
						m.response <- &TopicResponse{
							Topic:    CallBack,
							Response: NewResponse(TaskProcess, m.id, "", m.token, msg),
						}
					}()
				},
			},
			OnHealthFunc: func(msg []byte) {
				m.response <- &TopicResponse{
					Topic:    CallBack,
					Response: NewResponse(Health, m.id, "", m.token, msg),
				}
			},
		},
	}
}
func (m *Manager) Start(ctx context.Context) error {
	childCtx, cancelFunc := context.WithCancel(ctx)
	m.ctx = childCtx
	m.cancel = cancelFunc
	m.reader = append(m.reader, NewReader[*Request](m.ctx, m.address, fmt.Sprintf("palm-%s", uuid.NewString()), ManagerTopic, m.config.KafkaConfig))
	//m.reader = append(m.reader, NewReader[*Request](m.ctx, m.address, "palm-task", TaskTopic, m.config.KafkaConfig))
	m.writer = NewWriter(m.ctx, m.address, m.config.KafkaConfig)
	go func() {
		for _, reader := range m.reader {
			_reader := reader
			m.wg.Add(1)
			go func() {
				defer m.wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case m.message <- <-_reader.ReadMessage(ctx):
					}
				}
			}()
		}
		m.wg.Wait()
		close(m.message)
	}()
	m.config.OnConnectBeforeFunc("", "start agent")
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
		select {
		case <-m.ctx.Done():
			return
		default:
			if request.Token != "" {
				if m.token != request.Token {
					continue
				}
				m.processMessage(request)
			}
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
		m.agent.AddTask(request.Id, &req)
	case ManagerRequest:

	default:
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
	return m
}

func (m *Manager) Finish() {
	m.ctx.Done()
}
