package dnslog

import (
	"bytes"
	"context"
	"fmt"
	dnslogbrokers "github.com/yaklang/yaklang/common/cybertunnel/dnslog/brokers"
	"io"
	"net"
	"strings"
	"time"

	"github.com/ReneKroon/ttlcache"

	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func fetchExternalIP() (net.IP, error) {
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
		raw, err := io.ReadAll(rsp.Body)
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

type DNSLogGRPCServer struct {
	tpb.DNSLogServer

	ExternalIP       string
	domain           string
	cache            *ttlcache.Cache
	tokenToModeCache *ttlcache.Cache
	core             *facades.DNSServer
}

func (D *DNSLogGRPCServer) RequireDomain(ctx context.Context, params *tpb.RequireDomainParams) (*tpb.RequireDomainResponse, error) {
	var mode = params.GetMode()
	if mode == "*" {
		mode = dnslogbrokers.Random()
	}
	var a, _ = dnslogbrokers.Get(mode)
	if a != nil {
		domain, token, err := a.Require(30*time.Second, "http://192.168.3.113:9999")
		if err != nil {
			return nil, utils.Errorf("require[%v] dnslog failed: %s", mode, err)
		}
		D.tokenToModeCache.Set(token, a.Name())
		return &tpb.RequireDomainResponse{
			Domain: domain,
			Token:  token,
			Mode:   a.Name(),
		}, nil
	}
	token := utils.RandStringBytes(10)
	token = strings.ToLower(token)
	return &tpb.RequireDomainResponse{
		Domain: fmt.Sprintf("%v.%v", token, D.domain),
		Token:  token,
		Mode:   "default",
	}, nil
}

func (D *DNSLogGRPCServer) QueryExistedDNSLog(ctx context.Context, params *tpb.QueryExistedDNSLogParams) (*tpb.QueryExistedDNSLogResponse, error) {
	mode := params.GetMode()
	if mode == "*" {
		mode = dnslogbrokers.Random()
	}

	if mode == "" {
		raw, _ := D.tokenToModeCache.Get(params.GetToken())
		ret := utils.InterfaceToString(raw)
		if ret != "" {
			mode = ret
		}
	}
	if mode != "" {
		var a, _ = dnslogbrokers.Get(params.Mode)
		if a != nil {
			results, err := a.GetResult(params.GetToken(), 30*time.Second, "http://192.168.3.113:9999")
			if err != nil {
				return nil, utils.Errorf("require[%v] dnslog failed: %s", a.Name(), err)
			}
			return &tpb.QueryExistedDNSLogResponse{
				Events: results,
			}, nil
		}
	}

	raw, ok := D.cache.Get(params.GetToken())
	if !ok {
		return &tpb.QueryExistedDNSLogResponse{Events: nil}, nil
	}
	events := &tpb.QueryExistedDNSLogResponse{Events: raw.([]*tpb.DNSLogEvent)}
	return events, nil
}

func NewDNSLogServer(domain string, externalIP string) (*DNSLogGRPCServer, error) {
	ip := externalIP
	if externalIP == "" {
		ipIns, err := fetchExternalIP()
		if err != nil {
			return nil, err
		}
		ip = ipIns.String()
	}

	coreDNSServer, err := facades.NewDNSServer(domain, ip, "0.0.0.0", 53)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			err := coreDNSServer.Serve(context.Background())
			if err != nil {
				log.Errorf("DNSServer serve failed: %v", err)
			}
			time.Sleep(time.Second)
		}
	}()
	cache := ttlcache.NewCache()
	cache.SetTTL(24 * time.Hour)
	tokenToModeCache := ttlcache.NewCache()
	cache.SetTTL(24 * time.Hour)

	coreDNSServer.SetCallback(func(i *facades.VisitorLog) {
		tokenRaw, ok := i.Details["token"]
		token := fmt.Sprint(tokenRaw)
		event := &tpb.DNSLogEvent{
			Type:       utils.MapGetString(i.Details, "dns-type"),
			Token:      token,
			Domain:     strings.Trim(utils.MapGetString(i.Details, "domain"), "."),
			RemoteAddr: strings.Trim(utils.MapGetString(i.Details, "remote-addr"), " "),
			Raw:        []byte(utils.MapGetString(i.Details, "raw")),
			Timestamp:  time.Now().Unix(),
		}
		if event.RemoteAddr != "" {
			host, port, _ := utils.ParseStringToHostPort(event.RemoteAddr)
			event.RemoteIP = host
			event.RemotePort = int32(port)
		}
		if ok {
			log.Infof("token: %v", token)
			resultsRaw, existed := cache.Get(token)
			if !existed {
				cache.Set(token, []*tpb.DNSLogEvent{event})
				return
			}
			result := resultsRaw.([]*tpb.DNSLogEvent)
			result = append(result, event)
			cache.Set(token, result)
		} else {

		}
	})

	grpcServe := &DNSLogGRPCServer{
		ExternalIP:       externalIP,
		domain:           domain,
		cache:            cache,
		tokenToModeCache: tokenToModeCache,
		core:             coreDNSServer,
	}
	return grpcServe, nil
}
