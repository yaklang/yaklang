package kafka

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/kafka/health"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"sync/atomic"
	"time"
)

type AgentType int

var agentLogger = log.GetLogger("agent")

const (
	ScanAgent AgentType = iota + 1
)

type AgentConfig struct {
	debug               bool
	heathTimeout        int
	OnConnectBeforeFunc func(nodeId string, agent *Agent)
	OnConnectAfterFunc  func(nodeId string, agent *Agent)
	OnCloseBeforeFunc   func(nodeId string, agent *Agent)
	OnCloseAfterFunc    func(nodeId string, agent *Agent)
}

func defaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		heathTimeout: 1,
		OnConnectBeforeFunc: func(nodeId string, agent *Agent) {
			log.Infof("pre connect kafka: %s", agent.address)
		},
		OnConnectAfterFunc: func(nodeId string, agent *Agent) {
			agentLogger.Infof("success connect. node id is: %s,node token is: %s", nodeId, agent.Token)
		},
		OnCloseBeforeFunc: func(nodeId string, agent *Agent) {
			agentLogger.Infof("node: %s,token: %s have closed", nodeId, agent.Token)
		},
		OnCloseAfterFunc: func(nodeId string, agent *Agent) {
			log.Infof("agent has closed")
		},
	}
}

type AgentConfigOptions func(config *AgentConfig)

func WithOnConnectBeforeFunc(_fun func(string, *Agent)) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.OnConnectBeforeFunc = _fun
	}
}
func WithOnConnectAfterFunc(_fun func(string, *Agent)) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.OnConnectAfterFunc = _fun
	}
}
func WithOnCloseBeforeFunc(_fun func(string, *Agent)) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.OnCloseBeforeFunc = _fun
	}
}
func WithOnCloseAfterFunc(_fun func(string, *Agent)) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.OnCloseAfterFunc = _fun
	}
}
func WithAgentDebug(debug bool) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.debug = debug
	}
}
func WithHealthTimeout(timeout int) AgentConfigOptions {
	return func(config *AgentConfig) {
		config.heathTimeout = timeout
	}
}

type Agent struct {
	id       string
	Token    string
	address  string
	ctx      context.Context
	cancel   context.CancelFunc
	_type    AgentType
	mux      *sync.Mutex
	status   *atomic.Int64
	manager  *TaskManager
	config   *AgentConfig
	messages chan *Request
	reader   []*AgentReader[*Request]
	writer   *AgentWriter
	cache    *utils.CacheWithKey[string, *Request]
	//上下文取消之后，未处理的请求，当上下文恢复之后，可能需要对原有的请求进行处理，待定
	uncompletedRequest utils.SafeMap[*Request]

	agentEnvironment *health.SystemMatrix //运行环境
	//记录当前agent中需要去哪些Topic中读取，并记录了reader
	topicManager *TopicManager
	//记录执行过程中的logger
	logger *bytes.Buffer
}

func newAgentEx(id string, address string, typ AgentType, ctx context.Context, opts ...AgentConfigOptions) (*Agent, error) {
	if address == "" {
		return nil, utils.Error("remote url is null,check it")
	}
	environment, err := health.NewSystemMatrixBase()
	if err != nil {
		return nil, errors.Join(err, utils.Errorf("get agent environment fail: %s", err))
	}
	childCtx, cancelFunc := context.WithCancel(ctx)
	config := defaultAgentConfig()
	for _, opt := range opts {
		opt(config)
	}
	logBuffer := bytes.NewBuffer(nil)
	agentLogger.AddOutput(logBuffer)
	topicManager := newTopicManagerFromAgentType(typ)
	readers := topicManager.GenerateTopicReader(ctx, address)
	writer := NewWriter(ctx, address)
	agent := &Agent{
		id:                 id,
		address:            address,
		_type:              typ,
		ctx:                childCtx,
		cancel:             cancelFunc,
		mux:                &sync.Mutex{},
		status:             &atomic.Int64{},
		config:             config,
		messages:           make(chan *Request, 1024),
		cache:              utils.NewTTLCacheWithKey[string, *Request](time.Second * time.Duration(60)),
		reader:             readers,
		writer:             writer,
		Token:              uuid.NewString(),
		uncompletedRequest: *utils.NewSafeMap[*Request](),
		agentEnvironment:   environment,
		logger:             logBuffer,
	}
	manager := NewTaskManager(ctx)
	agent.manager = manager
	return agent, nil
}
func (a *Agent) Start() error {
	var wg = &sync.WaitGroup{}
	if a.config.OnConnectBeforeFunc != nil {
		a.config.OnConnectBeforeFunc(a.id, a)
	}
	//发送注册消息，只是消息类型的不同
	if err := a.WriteRegisterMessage(); err != nil {
		a.ShutDown()
		return err
	}
	ticker := time.NewTicker(time.Duration(a.config.heathTimeout) * time.Second)
	a.status.Store(1)
	if a.config.OnConnectAfterFunc != nil {
		a.config.OnConnectAfterFunc(a.id, a)
	}
	go func() {
		select {
		case <-a.ctx.Done():
			wg.Wait()
			close(a.messages)
			for message := range a.messages {
				a.uncompletedRequest.Set(message.RequestId, message)
			}
		case <-ticker.C:
			a.writeHeathMessage()
		}
	}()
	for _, reader := range a.reader {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-a.ctx.Done():
					return
				case a.messages <- <-reader.ReadMessage(a.ctx):
				}
			}
		}()
	}
	return nil
}
func (a *Agent) ReStart() {
	//恢复未处理的消息
	a.messages = make(chan *Request, 1024)
	go func() {
		for message := range a.messages {
			a.messages <- message
		}
	}()
	a.Start()
}
func (a *Agent) ShutDown() {
	if a.config.OnCloseBeforeFunc != nil {
		a.config.OnCloseBeforeFunc(a.id, a)
	}
	a.ctx.Done()
	if a.config.OnCloseAfterFunc != nil {
		a.config.OnCloseAfterFunc(a.id, a)
	}
}
func (a *Agent) writeHeathMessage() error {
	bytes, _ := json.Marshal(a.agentEnvironment)
	request := newRequest(Heart, a.id, a.Token, bytes)
	return a.writer.WriteRequest(request, heartTopic)
}
func (a *Agent) WriteRegisterMessage() error {
	request := newRequest(Register, a.id, a.Token, nil)
	return a.writer.WriteRequest(request, heartTopic)
}
func (a *Agent) WriteLog() error {
	i := a.logger.Bytes()
	if len(i) > 0 {
		request := newRequest(Log, a.id, a.Token, i)
		return a.writer.WriteLog(request)
	}
	return nil
}

func NewScanAgent(name string, address string, ctx context.Context, opts ...AgentConfigOptions) (*Agent, error) {
	return newAgentEx(name, address, ScanAgent, ctx, opts...)
}
