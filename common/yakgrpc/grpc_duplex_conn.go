package yakgrpc

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
)

var (
	serverPushMutex    = new(sync.Mutex)
	serverPushCallback []func(response *ypb.DuplexConnectionResponse)
)

func (s *Server) DuplexConnection(stream ypb.Yak_DuplexConnectionServer) error {
	
}
