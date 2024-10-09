package cybertunnel

import (
	"context"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

func (t *TunnelServer) RequireHTTPRequestTrigger(ctx context.Context, req *tpb.RequireHTTPRequestTriggerParams) (*tpb.RequireHTTPRequestTriggerResponse, error) {
	if defaultHTTPTrigger == nil {
		return nil, utils.Error("http trigger not started")
	}
	var token = strings.ToLower(utils.RandStringBytes(12))
	results, err := defaultHTTPTrigger.Register(token, func(bytes []byte) []byte {
		rsp := req.GetExpectedHTTPResponse()
		if len(rsp) > 0 {
			return rsp
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	})
	if err != nil {
		return nil, err
	}
	rsp := &tpb.RequireHTTPRequestTriggerResponse{}
	rsp.Token = token
	for _, i := range results {
		if strings.HasPrefix(i, "https://") {
			rsp.Urls = append(rsp.Urls, i)
		} else if strings.HasPrefix(i, "http://") {
			if rsp.PrimaryUrl == "" {
				rsp.PrimaryUrl = i
			}
			rsp.Urls = append(rsp.Urls, i)
		} else {
			if rsp.PrimaryHost == "" {
				rsp.PrimaryHost = i
			}
			rsp.Hosts = append(rsp.Hosts, i)
		}
	}
	return rsp, nil
}

func (t *TunnelServer) QueryExistedHTTPRequestTrigger(ctx context.Context, req *tpb.QueryExistedHTTPRequestTriggerRequest) (*tpb.QueryExistedHTTPRequestTriggerResponse, error) {
	if defaultHTTPTrigger == nil {
		return nil, utils.Error("http trigger not started")
	}
	resp, ok := defaultHTTPTrigger.notificationCache.Get(strings.ToLower(req.GetToken()))
	if ok {
		for _, r := range resp {
			r.Timestamp = time.Now().Unix()
		}
		return &tpb.QueryExistedHTTPRequestTriggerResponse{
			Notifications: resp,
		}, nil
	}
	return &tpb.QueryExistedHTTPRequestTriggerResponse{
		Notifications: nil,
	}, nil
}
