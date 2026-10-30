package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/minimartian"
)

// Registration happens only after the main listener has bound successfully.
// Instance connections enter the same MITM pipeline as the main proxy socket.
func (s *Server) prepareBrowserMITMIngress(ctx context.Context, address string, auth bool) (crep.MITMConfig, func(), func()) {
	incoming := make(chan *minimartian.WrapperedConn, 16)
	var stop func()
	ready := func() {
		if s.browserBridge != nil {
			stop = s.browserBridge.RegisterMITMIngress(ctx, address, auth, incoming)
		}
	}
	cleanup := func() {
		if stop != nil {
			stop()
		}
	}
	return crep.MITM_SetExtraIncomingConnectionChannel(incoming), ready, cleanup
}
