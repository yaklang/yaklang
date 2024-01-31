package cybertunnel

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	tokenCache       = utils.NewTTLCache[int](time.Minute)
	portToTokenCache = utils.NewTTLCache[string](time.Minute)
)

func (t *TunnelServer) RequireRandomPortTrigger(ctx context.Context, req *tpb.RequireRandomPortTriggerParams) (*tpb.RequireRandomPortTriggerResponse, error) {
	var targetPort int
	for {
		port := rand.Intn(65534-55000) + 55000
		if !utils.IsTCPPortAvailable(port) {
			continue
		}
		_, ok := randomPortTrigger.localPort.Load(port)
		if ok {
			continue
		}

		_, ok = portToTokenCache.Get(fmt.Sprint(port))
		if ok {
			continue
		}
		targetPort = port
		break
	}
	rsp, err := t.RemoteIP(ctx, &tpb.Empty{})
	if err != nil {
		return nil, err
	}
	tokenCache.Set(req.GetToken(), targetPort)
	portToTokenCache.Set(fmt.Sprint(targetPort), req.GetToken())
	return &tpb.RequireRandomPortTriggerResponse{
		Port:       int32(targetPort),
		Token:      req.GetToken(),
		ExternalIP: rsp.GetIPAddress(),
	}, nil
}

func (t *TunnelServer) QueryExistedRandomPortTrigger(c context.Context, req *tpb.QueryExistedRandomPortTriggerRequest) (*tpb.QueryExistedRandomPortTriggerResponse, error) {
	localPort, ok := tokenCache.Get(req.GetToken())
	if ok {
		notif, err := randomPortTrigger.GetTriggerNotification(localPort)
		if err != nil {
			return nil, err
		}
		host, port, _ := utils.ParseStringToHostPort(notif.CurrentRemoteAddr)

		var events []*tpb.RandomPortTriggerEvent
		event := &tpb.RandomPortTriggerEvent{
			RemoteAddr:                            notif.CurrentRemoteAddr,
			RemoteIP:                              host,
			RemotePort:                            int32(port),
			LocalPort:                             int32(localPort),
			History:                               notif.Histories,
			CurrentRemoteCachedConnectionCount:    int32(notif.CurrentRemoteCachedConnectionCount),
			LocalPortCachedHistoryConnectionCount: int32(notif.LocalPortCachedHistoryConnectionCount),
			TriggerTimestamp:                      notif.TriggerTimestamp,
			Timestamp:                             notif.Timestamp,
		}
		events = append(events, event)

		return &tpb.QueryExistedRandomPortTriggerResponse{Events: events}, nil
	}
	return nil, utils.Errorf("empty token port mapped")
}
