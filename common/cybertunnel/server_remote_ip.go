package cybertunnel

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"yaklang.io/yaklang/common/cybertunnel/tpb"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
)

func FetchExternalIP() (net.IP, error) {
	dailer := utils.NewDefaultHTTPClient()
	for _, domain := range []string{
		"ifconfig.me",
		"ipinfo.io/ip",
		"ipecho.net/plain",
		"www.trackip.net/ip",
		"ip.sb",
		"v4.ident.me",
		"ident.me",
	} {
		u := fmt.Sprintf("http://%s", domain)
		rsp, err := dailer.Get(u)
		if err != nil {
			log.Errorf("fetch %s failed: %s", u, err)
			continue
		}
		raw, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			log.Errorf("read body failed: %s", err)
			continue
		}
		raw = bytes.TrimSpace(raw)
		ip := net.ParseIP(utils.FixForParseIP(string(raw)))
		if ip != nil {
			return ip, nil
		}
	}
	return nil, utils.Error("cannot fetch external ip...")
}

func (s *TunnelServer) RemoteIP(ctx context.Context, req *tpb.Empty) (*tpb.RemoteIPResponse, error) {
	// ifconfig.me
	// ipinfo.io/ip
	// ipecho.net/plain
	// www.trackip.net/ip
	// ip.sb
	// v4.ident.me
	// ident.me
	if s.ExternalIP != "" {
		return &tpb.RemoteIPResponse{IPAddress: s.ExternalIP}, nil
	}

	ip, err := FetchExternalIP()
	if err != nil {
		return nil, err
	}
	return &tpb.RemoteIPResponse{IPAddress: ip.String()}, nil
}
