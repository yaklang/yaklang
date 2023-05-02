package node

import (
	"context"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"github.com/tevino/abool"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/mq"
	"yaklang/common/spec"
	"yaklang/common/utils/healthinfo"
	"yaklang/common/yak"
)

type NodeBase struct {
	rootCtx context.Context
	cancel  context.CancelFunc

	NodeType      spec.NodeType
	NodeId        string
	WebServerPort string
	ExternalIp    string
	token         string

	healthManager *healthinfo.Manager

	rpcExchange string
	rpcServer   *mq.RPCServer
	publisher   *mq.Publisher
	rpcClient   *mq.RPCClient
	broker      *mq.Broker

	// map[string]*tickerFunc
	tickerFuncs *sync.Map

	// 表示是否注册成功
	isRegistered *abool.AtomicBool

	// []func()
	//    当成功注册之后会执行这个函数
	afterRegisterFuncs []func()

	// 接受通知的处理函数
	onNotificationComingFuncs []func(msg *amqp.Delivery)

	// 脚本执行引擎
	ScriptExecutor *yak.ScriptEngine

	//
}

func (n *NodeBase) HookOnNotificationComingHandler(f ...func(msg *amqp.Delivery)) {
	n.onNotificationComingFuncs = append(n.onNotificationComingFuncs, f...)
}

func (n *NodeBase) IsRegistered() bool {
	return n.isRegistered.IsSet()
}

func (n *NodeBase) HookAfterRegisteringFinished(fs ...func()) {
	n.afterRegisterFuncs = append(n.afterRegisterFuncs, fs...)
}

func (n *NodeBase) GetRPCServer() *mq.RPCServer {
	return n.rpcServer
}

func (n *NodeBase) GetRPCClient() *mq.RPCClient {
	return n.rpcClient
}

func (n *NodeBase) WithCancelContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(n.rootCtx)
}

func NewNodeBase(nodeType spec.NodeType, exchange, id string, token string, configs ...mq.BrokerConfigHandler) (*NodeBase, error) {
	ctx, cancel := context.WithCancel(context.Background())

	node := &NodeBase{
		NodeType:       nodeType,
		rootCtx:        ctx,
		cancel:         cancel,
		NodeId:         id,
		token:          token,
		rpcExchange:    exchange,
		tickerFuncs:    new(sync.Map),
		isRegistered:   abool.NewBool(false),
		ScriptExecutor: yak.NewScriptEngine(200),
	}

	//
	configs = append(configs, node.getSyncConfigMqHandler()...)

	err := node.init(configs...)
	if err != nil {
		return nil, errors.Errorf("node init failed: %v", err)
	}

	return node, nil
}

func (n *NodeBase) DoConfigure(configs ...mq.BrokerConfigHandler) {
	n.rpcServer.DoConfigure(configs...)
}

func (n *NodeBase) init(configs ...mq.BrokerConfigHandler) (err error) {
	n.healthManager, err = healthinfo.NewHealthInfoManager(10*time.Second, 10*time.Minute)
	if err != nil {
		return errors.Errorf("build health manager failed: %v", err)
	}

	n.rpcServer, err = mq.NewRPCServer(
		n.rootCtx, n.rpcExchange, n.NodeId,
		configs...,
	)
	if err != nil {
		return errors.Errorf("build rpc server failed[%v]: %v", n.NodeId, err)
	}

	n.rpcClient, err = n.rpcServer.GetRPCClient(n.NodeId)
	if err != nil {
		return errors.Errorf("build rpc client failed[%v]: %v", n.NodeId, err)
	}

	n.broker = n.rpcServer.GetBroker()
	n.publisher = n.broker.GetPublisher()

	n.initScriptEngine()
	n.HookOnNotificationComingHandler(n.onScriptTask)

	log.Info("start to register basic node manager api service")
	n.initBasicNodeManagerAPI()

	return nil
}

func (n *NodeBase) Serve() {
	go n.rpcServer.Serve()

	for {
		time.Sleep(500 * time.Millisecond)
		if n.broker.IsServing() {
			break
		}
	}

	go n.startDaemon()
	select {
	case <-n.rootCtx.Done():
	}
}

func (n *NodeBase) Shutdown() {
	n.cancel()
}

func (n *NodeBase) startDaemon() {
	tick := time.Tick(10 * time.Second)
	tick1s := time.Tick(1 * time.Second)

	for {
		if err := n.register(); err != nil {
			log.Errorf("register failed: %v, retry in 3s", err)
			time.Sleep(3 * time.Second)
			continue
		} else {
			n.heartbeat()
			break
		}
	}
	n.isRegistered.Set()
	for _, f := range n.afterRegisterFuncs {
		f()
	}

	for {
		select {
		case <-tick:
			n.heartbeat()
		case <-tick1s:
			n.WalkTickerFunc(func(name string, f *tickerFunc) {
				if f.first && !f.firstExecuted.IsSet() {
					f.firstExecuted.Set()
					f.F()
					return
				}

				f.currentMod = (f.currentMod + 1) % f.IntervalSeconds
				if f.currentMod == 0 {
					f.F()
				}
			})
		}
	}
}

func (n *NodeBase) register() error {

	ctx, _ := context.WithTimeout(n.rootCtx, 5*time.Second)
	body, err := n.rpcClient.Call(ctx, spec.API_RegisterNode, spec.ServerNodeId,
		&spec.NodeRegisterRequest{
			NodeId:    n.NodeId,
			Token:     n.token,
			NodeType:  n.NodeType,
			Timestamp: time.Now().Unix(),
		})
	if err != nil {
		return errors.Errorf("call register failed: %v", err)
	}

	var rsp spec.NodeRegisterResponse
	err = json.Unmarshal(body, &rsp)
	if err != nil {
		return errors.Errorf("marshal failed: %v", err)
	}

	if !rsp.Ok {
		return errors.Errorf("register failed: %v", rsp.Reason)
	}

	n.token = rsp.Token
	n.WebServerPort = rsp.WebServerPort
	log.Infof("nodeid=%v token=%v", n.NodeId, rsp.Token)

	return nil
}

func (n *NodeBase) NewBaseMessage(typeInfo spec.MessageType) *spec.Message {
	return &spec.Message{
		NodeId:    n.NodeId,
		Token:     n.token,
		Type:      typeInfo,
		Timestamp: time.Now().Unix(),
	}
}

func (n *NodeBase) GetToken() string {
	return n.token
}

func (n *NodeBase) GetRootContext() context.Context {
	return n.rootCtx
}
