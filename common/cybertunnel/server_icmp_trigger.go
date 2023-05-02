package cybertunnel

import (
	"context"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
)

func (s *TunnelServer) QuerySpecificICMPLengthTrigger(ctx context.Context, req *tpb.QuerySpecificICMPLengthTriggerParams) (*tpb.QuerySpecificICMPLengthTriggerResponse, error) {
	rsp, err := icmpTrigger.GetICMPTriggerNotification(int(req.Length))
	if err != nil {
		return nil, utils.Errorf("call icmp trigger.GetICMPTriggerNotification failed: %s", err)
	}
	return &tpb.QuerySpecificICMPLengthTriggerResponse{
		Notifications: []*tpb.ICMPTriggerNotification{
			{
				Size:                               int32(rsp.Size),
				CurrentRemoteAddr:                  rsp.CurrentRemoteAddr,
				Histories:                          rsp.Histories,
				CurrentRemoteCachedConnectionCount: int32(rsp.CurrentRemoteCachedConnectionCount),
				SizeCachedHistoryConnectionCount:   int32(rsp.SizeCachedHistoryConnectionCount),
				TriggerTimestamp:                   rsp.TriggerTimestamp,
				Timestamp:                          rsp.Timestamp,
			},
		},
	}, nil
}
