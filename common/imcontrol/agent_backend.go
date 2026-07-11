package imcontrol

import (
	"context"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AIReActStream 是 IM Engine 需要的 AIReAct 双向流最小接口。
type AIReActStream interface {
	Send(*ypb.AIInputEvent) error
	Recv() (*ypb.AIOutputEvent, error)
	CloseSend() error
}

// AIReActStreamFactory 创建 AIReAct 双向流。
type AIReActStreamFactory interface {
	StartAIReAct(context.Context) (AIReActStream, error)
}

// AISessionStore 提供 IM 控制面板需要的 AI Session 读写能力。
type AISessionStore interface {
	QueryAISession(context.Context, *ypb.QueryAISessionRequest) (*ypb.QueryAISessionResponse, error)
	UpdateAISessionIMMeta(context.Context, *ypb.UpdateAISessionIMMetaRequest) (*ypb.DbOperateMessage, error)
}
