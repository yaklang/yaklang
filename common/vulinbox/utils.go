package vulinbox

import (
	"context"
	grpcMetadata "google.golang.org/grpc/metadata"
	"yaklang/common/yakgrpc"
	"yaklang/common/yakgrpc/ypb"
)

func new_EmptyServer() *yakgrpc.Server {
	return &yakgrpc.Server{}
}

type VirtualYakExecServer struct {
	send func(result *ypb.ExecResult) error
}

func (v *VirtualYakExecServer) Send(result *ypb.ExecResult) error {
	if v.send == nil {
		panic("not set sender")
	}
	return v.send(result)
}

func (v *VirtualYakExecServer) SetHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) SendHeader(md grpcMetadata.MD) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) SetTrailer(md grpcMetadata.MD) {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) Context() context.Context {
	return context.Background()
}

func (v *VirtualYakExecServer) SendMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func (v *VirtualYakExecServer) RecvMsg(m interface{}) error {
	//TODO implement me
	panic("implement me")
}

func NewVirtualYakExecServerWithMessageHandle(h func(result *ypb.ExecResult) error) *VirtualYakExecServer {
	return &VirtualYakExecServer{send: h}
}
