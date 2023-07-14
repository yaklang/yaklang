package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	vulinboxAgentMap = new(sync.Map)
)

func GetVulinboxAgent(addr string) (*VulinboxAgentFacade, bool) {
	raw, ok := vulinboxAgentMap.Load(addr)
	if !ok {
		return nil, false
	}
	agent, ok := raw.(*VulinboxAgentFacade)
	return agent, ok
}

func RegisterVulinboxAgent(addr string, agent *VulinboxAgentFacade) {
	a, ok := GetVulinboxAgent(addr)
	if ok {
		a.Close()
	}
	vulinboxAgentMap.Store(addr, agent)
}

type VulinboxAgentFacade struct {
	addr         string
	closed       bool
	disconnect   func()
	requestCount int64
	pingCount    int64
	lastActiveAt int64
}

func (v *VulinboxAgentFacade) Close() {
	if v == nil {
		return
	}
	if v.disconnect != nil {
		v.disconnect()
	}
}

func (v *VulinboxAgentFacade) AddPing() {
	atomic.AddInt64(&v.pingCount, 1)
	v.lastActiveAt = time.Now().Unix()
}

func (v *VulinboxAgentFacade) AddRequestCount() {
	atomic.AddInt64(&v.requestCount, 1)
	v.lastActiveAt = time.Now().Unix()
}

func (v *VulinboxAgentFacade) IsClosed() bool {
	if v == nil {
		return true
	}
	return v.closed
}

func (v *VulinboxAgentFacade) Status() *ypb.IsRemoteAddrAvailableResponse {
	if v.IsClosed() {
		return &ypb.IsRemoteAddrAvailableResponse{
			Addr:         v.addr,
			IsAvailable:  false,
			Reason:       "connection is closed...",
			Status:       "offline",
			PingCount:    v.pingCount,
			RequestCount: v.requestCount,
			LastActiveAt: v.lastActiveAt,
		}
	}
	return &ypb.IsRemoteAddrAvailableResponse{
		Addr:         v.addr,
		IsAvailable:  true,
		Status:       "offline",
		PingCount:    v.pingCount,
		RequestCount: v.requestCount,
		LastActiveAt: v.lastActiveAt,
	}
}

func (s *Server) DisconnectVulinboxAgent(ctx context.Context, req *ypb.DisconnectVulinboxAgentRequest) (*ypb.Empty, error) {
	ins, ok := GetVulinboxAgent(req.GetAddr())
	if !ok {
		return &ypb.Empty{}, nil
	}
	ins.Close()
	vulinboxAgentMap.Delete(req.GetAddr())
	return &ypb.Empty{}, nil
}

func (s *Server) GetRegisteredVulinboxAgent(ctx context.Context, req *ypb.GetRegisteredAgentRequest) (*ypb.GetRegisteredAgentResponse, error) {
	var infos []*ypb.IsRemoteAddrAvailableResponse
	vulinboxAgentMap.Range(func(key, value any) bool {
		info, ok := value.(*VulinboxAgentFacade)
		if !ok {
			return true
		}
		infos = append(infos, info.Status())
		return true
	})
	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].LastActiveAt-infos[j].LastActiveAt == 0 {
			return infos[i].Addr > infos[i].Addr
		} else {
			return infos[i].LastActiveAt > infos[j].LastActiveAt
		}
	})
	return &ypb.GetRegisteredAgentResponse{
		Agents: infos,
	}, nil
}

func (s *Server) ConnectVulinboxAgent(ctx context.Context, req *ypb.IsRemoteAddrAvailableRequest) (*ypb.IsRemoteAddrAvailableResponse, error) {
	return s.IsRemoteAddrAvailable(ctx, req)
}

func (s *Server) IsRemoteAddrAvailable(ctx context.Context, req *ypb.IsRemoteAddrAvailableRequest) (*ypb.IsRemoteAddrAvailableResponse, error) {
	if req.GetAddr() == "" {
		return nil, utils.Errorf("remote agent addr empty")
	}

	var addr = utils.AppendDefaultPort(req.GetAddr(), 8787)
	if addr == "" {
		return nil, utils.Errorf("remote agent addr empty")
	}

	agent, ok := GetVulinboxAgent(addr)
	if ok {
		return agent.Status(), nil
	}

	info := &VulinboxAgentFacade{}
	disconnectBox, err := lowhttp.ConnectVulinboxAgentEx(addr, func(request []byte) {
		info.AddRequestCount()
	}, func() {
		info.AddPing()
	}, func() {
		info.closed = true
	})
	if err != nil {
		return nil, utils.Errorf("connect to remove agent failed: %s", err)
	}
	info.disconnect = disconnectBox
	RegisterVulinboxAgent(addr, info)

	return &ypb.IsRemoteAddrAvailableResponse{
		Addr:         addr,
		IsAvailable:  true,
		Status:       "online",
		PingCount:    info.pingCount,
		RequestCount: info.requestCount,
	}, nil
}
