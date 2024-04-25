package cybertunnel

import (
	"context"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

var httpRequestCache = utils.NewTTLCache[string](time.Minute * 10)

func (t *TunnelServer) RequireHTTPRequestTrigger(ctx context.Context, req *tpb.RequireHTTPRequestTriggerParams) (*tpb.RequireHTTPRequestTriggerResponse, error) {
	return nil, nil
}

func (t *TunnelServer) QueryExistedHTTPRequestTrigger(ctx context.Context, req *tpb.QueryExistedHTTPRequestTriggerRequest) (*tpb.QueryExistedHTTPRequestTriggerResponse, error) {
	return nil, nil
}
