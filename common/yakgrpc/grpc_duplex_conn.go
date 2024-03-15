package yakgrpc

import (
	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) DuplexConnection(stream ypb.Yak_DuplexConnectionServer) error {
	id := uuid.New().String()
	yakit.RegisterServerPushCallback(id, stream)
	defer yakit.UnRegisterServerPushCallback(id)

	yakit.BroadcastData("global", map[string]any{
		"config": map[string]any{
			"enableServerPush": true,
		},
	})
	<-stream.Context().Done()
	return stream.Context().Err()
}
