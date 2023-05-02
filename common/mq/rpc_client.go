package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
	"github.com/tevino/abool"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

const (
	RPC_MessageType_Request         = "request"
	RPC_MessageType_Cancel          = "cancel"
	RPC_MessageType_Response        = "response"
	RPC_MessageType_Error           = "error"
	RPC_MessageType_RequestReceived = "req-recv"
)

type RPCClient struct {
	broker    *Broker
	publisher *Publisher
	exchange  string
	id        string

	requestSentTimeout time.Duration
	cache              *sync.Map
}

func (r *RPCClient) GetRequestSentTimeout() time.Duration {
	if r.requestSentTimeout <= 0 {
		r.requestSentTimeout = 5 * time.Second
		return r.GetRequestSentTimeout()
	}
	return r.requestSentTimeout
}

func (r *RPCClient) SetRequestSentTimeout(duration time.Duration) {
	if r.requestSentTimeout > 0 {
		r.requestSentTimeout = duration
	} else {
		r.requestSentTimeout = 5 * time.Second
	}
}

func (r *RPCClient) GetPublisher() *Publisher {
	return r.publisher
}

func NewRPCClient(ctx context.Context, exchange string, options ...BrokerConfigHandler) (*RPCClient, error) {
	broker, err := NewBroker(ctx, options...)
	if err != nil {
		return nil, errors.Errorf("build broker failed: %v", err)
	}

	id, err := uuid.NewV4()
	if err != nil {
		return nil, errors.Errorf("build uuid failed: %v", err)
	}

	return NewRPCClientWithBroker(broker, exchange, id.String())
}

func NewRPCClientWithBroker(broker *Broker, exchange string, id string) (*RPCClient, error) {
	r := &RPCClient{
		broker:    broker,
		publisher: broker.GetPublisher(),
		exchange:  exchange,
		cache:     new(sync.Map),
		id:        id,
	}

	broker.DoConfigure(
		RPCClientConfig(exchange, id, r.daemonCallback),
	)

	return r, nil
}

type rpcRequest struct {
	Uid        string
	Msg        *amqp.Publishing
	ctx        context.Context
	cancel     func()
	Rsp        *rpcResponse
	RoutingKey string

	// request sent
	haveRequestSent         *abool.AtomicBool
	haveRequestSentCtx      context.Context
	haveRequestSentFinished context.CancelFunc

	haveRspCtx  context.Context
	haveRspFunc context.CancelFunc
}

type rpcResponse struct {
	Buf    []byte
	Reason string
}

func (r *RPCClient) request(rootCtx context.Context, f, node string, req interface{}) (*rpcRequest, error) {
	if r == nil {
		return nil, utils.Errorf("RPCClient not initialed... for %v", node)
	}
	ctx, cancel := context.WithCancel(rootCtx)

	buf, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Errorf("Marshal json buf msg failed: %s", err)
	}

	uid, err := uuid.NewV4()
	if err != nil {
		return nil, errors.Errorf("get uuid4 failed: %s", err)
	}

	msg := amqp.Publishing{
		CorrelationId: uid.String(),
		Timestamp:     time.Now(),
		Type:          RPC_MessageType_Request,
		Body:          buf,
		AppId:         r.id,
	}

	rK := r.getRoutingKey(f, node)
	err = r.publisher.PublishTo(r.exchange, rK, msg)
	if err != nil {
		return nil, errors.Errorf("publish failed: %v", err)
	}

	rspCtx, rspFinished := context.WithCancel(ctx)
	ReqCtx, ReqCancel := context.WithCancel(context.Background())
	request := &rpcRequest{
		Uid: uid.String(), ctx: ctx, cancel: cancel,
		Msg: &msg, RoutingKey: r.getRoutingKey(f, node),

		// 用来标注 rpc 状态
		haveRequestSent:         abool.NewBool(false),
		haveRequestSentCtx:      ReqCtx,
		haveRequestSentFinished: ReqCancel,

		haveRspCtx:  rspCtx,
		haveRspFunc: rspFinished,
	}
	r.cache.Store(uid.String(), request)

	//ddl, ok := ctx.Deadline()
	//if !ok {
	//	return request, nil
	//}

	go func() {
		select {
		case <-rootCtx.Done():
			//case <-time.After(ddl.Sub(time.Now())):
		}
		r.cancel(uid.String())
	}()

	return request, nil
}

func (r *RPCClient) cancel(id string) {
	req, ok := r.cache.Load(id)
	if !ok {
		return
	}

	defer func() {
		go func() {
			select {
			case <-time.After(10 * time.Second):
				r.cache.Delete(id)
			}
		}()
	}()

	request := req.(*rpcRequest)
	err := r.publisher.PublishTo(r.exchange, request.RoutingKey, amqp.Publishing{
		CorrelationId: id,
		Timestamp:     time.Now(),
		Type:          RPC_MessageType_Cancel,
		Body:          []byte("client cancel"),
		AppId:         r.id,
	})
	if err != nil {
		log.Errorf("cancel msg pub failed: %v", err)
	}

	//select {
	//case <-request.ctx.Done():
	//	return
	//default:
	//	err := r.publisher.PublishTo(r.exchange, request.RoutingKey, amqp.Publishing{
	//		CorrelationId: id,
	//		Timestamp:     time.Now(),
	//		Type:          RPC_MessageType_Cancel,
	//		Body:          []byte("client cancel"),
	//		AppId:         r.id,
	//	})
	//	if err != nil {
	//		log.Errorf("cancel msg pub failed: %v", err)
	//	}
	//}
}

func (r *RPCClient) getRoutingKey(f, node string) string {
	return fmt.Sprintf("rpc.%v.%v", f, node)
}

func (r *RPCClient) Call(ctx context.Context, f, node string, req interface{}) ([]byte, error) {
	request, err := r.request(ctx, f, node, req)
	if err != nil {
		return nil, errors.Errorf("[Call] request failed: %v", err)
	}

	reqSentCtx, cancel := context.WithTimeout(request.ctx, r.GetRequestSentTimeout())
	_ = cancel
	select {
	case <-reqSentCtx.Done():
		return nil, utils.Errorf("wait request sent timeout event failed: %s", reqSentCtx.Err())
	case <-request.haveRequestSentCtx.Done():
		log.Infof("request sent to server: waiting for response: %s", request.Uid)
	}

	response, err := r.waitForResponse(request)
	if err != nil {
		return nil, errors.Errorf("wait for response failed: %v", err)
	}

	if response.Reason != "" {
		return nil, errors.Errorf("server error: %v", response.Reason)
	}

	return response.Buf, nil
}

func (r *RPCClient) waitForResponse(req *rpcRequest) (*rpcResponse, error) {
	select {
	case <-req.haveRspCtx.Done():
		if req.Rsp != nil {
			return req.Rsp, nil
		}
		return nil, errors.Errorf("context done")
	}
}

func (r *RPCClient) daemonCallback(requestId string, msg *amqp.Delivery) {
	req, ok := r.cache.Load(requestId)
	if !ok {
		log.Warnf("cannot found request id: %v", requestId)
		return
	}

	request := req.(*rpcRequest)
	switch msg.Type {
	case RPC_MessageType_RequestReceived:
		request.haveRequestSentFinished()
		request.haveRequestSent.Set()
	case RPC_MessageType_Response:
		request.Rsp = &rpcResponse{
			Buf: msg.Body,
		}
		request.haveRspFunc()
	case RPC_MessageType_Error:
		request.Rsp = &rpcResponse{
			Buf:    nil,
			Reason: string(msg.Body),
		}
		request.haveRspFunc()
	default:
		request.Rsp = &rpcResponse{
			Buf:    nil,
			Reason: fmt.Sprintf("client recv error type of msg: %v", msg.Type),
		}
		request.haveRspFunc()
		return
	}

}

func (r *RPCClient) Connect() error {
	if r.broker.IsServing() {
		return nil
	}
	return r.broker.RunBackground()
}
