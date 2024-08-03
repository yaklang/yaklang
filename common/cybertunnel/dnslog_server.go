package cybertunnel

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"time"

	dnslogbrokers "github.com/yaklang/yaklang/common/cybertunnel/brokers"

	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/facades"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var DefaultExternalIP *net.IP

func GetExternalIP() (net.IP, error) {
	if DefaultExternalIP != nil {
		return *DefaultExternalIP, nil
	}
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
			DefaultExternalIP = &ip
			return ip, nil
		}
	}
	return nil, utils.Error("cannot fetch external ip...")
}

type DNSLogGRPCServer struct {
	tpb.DNSLogServer

	ExternalIP       string
	domain           []string
	cache            *utils.Cache[[]*tpb.DNSLogEvent]
	tokenToModeCache *utils.Cache[string]
	core             *facades.DNSServer
}

func tryRegisterHTTPTrigger(token string) {
	defaultHTTPTrigger.Register(token, func(raw []byte) []byte {
		return []byte("HTTP/1.1 200 OK\r\n" +
			"Content-Length: " + fmt.Sprint(len(token)) + "\r\n" +
			"\r\n" + token)
	})
}

func (D *DNSLogGRPCServer) RequireDomain(ctx context.Context, params *tpb.RequireDomainParams) (*tpb.RequireDomainResponse, error) {
	mode := params.GetMode()
	if mode == "*" {
		mode = dnslogbrokers.Random()
	}

	a, _ := dnslogbrokers.Get(mode)
	if a != nil {
		domain, token, err := a.Require(15 * time.Second)
		if err != nil {
			return nil, utils.Errorf("require[%v] dnslog failed: %s", mode, err)
		}
		D.tokenToModeCache.Set(token, a.Name())
		tryRegisterHTTPTrigger(token)
		return &tpb.RequireDomainResponse{
			Domain: domain,
			Token:  token,
			Mode:   a.Name(),
		}, nil
	}

	if len(utils.StringArrayFilterEmpty(D.domain)) == 0 {
		return nil, utils.Errorf("no domain available")
	}

	token := utils.RandStringBytes(10)
	token = strings.ToLower(token)
	tryRegisterHTTPTrigger(token)

	defaultDomain := D.domain[0]
	if len(D.domain) > 1 {
		idx := rand.Intn(len(D.domain))
		defaultDomain = D.domain[idx]
	}

	return &tpb.RequireDomainResponse{
		Domain: fmt.Sprintf("%v.%v", token, defaultDomain),
		Token:  token,
		Mode:   "default",
	}, nil
}

func (D *DNSLogGRPCServer) QueryExistedDNSLog(ctx context.Context, params *tpb.QueryExistedDNSLogParams) (*tpb.QueryExistedDNSLogResponse, error) {
	mode := params.GetMode()
	if mode == "*" {
		mode = dnslogbrokers.Random()
	}

	var token = params.GetToken()
	httpResults, _ := defaultHTTPTrigger.QueryResults(strings.ToLower(params.GetToken()))

	if mode == "" {
		raw, _ := D.tokenToModeCache.Get(params.GetToken())
		ret := utils.InterfaceToString(raw)
		if ret != "" {
			mode = ret
		}
	}

	mergeResults := func(results []*tpb.DNSLogEvent) []*tpb.DNSLogEvent {
		var extraEvents = make([]*tpb.DNSLogEvent, len(results), len(results)+len(httpResults)+len(httpResults))
		copy(extraEvents, results)
		if len(httpResults) > 0 {
			for _, item := range httpResults {
				var t string
				if item.IsHttps {
					t = "HTTPS"
				} else {
					t = "HTTP"
				}
				_ = t
				domain := utils.ExtractHost(item.Url)
				ip, port, _ := utils.ParseStringToHostPort(item.GetRemoteAddr())
				event := &tpb.DNSLogEvent{
					Type:       t,
					Token:      token,
					Domain:     domain,
					RemoteAddr: item.GetRemoteAddr(),
					RemoteIP:   ip,
					RemotePort: int32(port),
					Raw:        item.GetRequest(),
					Timestamp:  item.GetTriggerTimestamp(),
					Mode:       mode,
				}
				eventCompact := &tpb.DNSLogEvent{
					Type:       "A",
					Token:      token,
					Domain:     domain,
					RemoteAddr: item.GetRemoteAddr(),
					RemoteIP:   ip,
					RemotePort: int32(port),
					Raw:        item.GetRequest(),
					Timestamp:  item.GetTriggerTimestamp(),
					Mode:       mode,
				}
				extraEvents = append(extraEvents, event)
				extraEvents = append(extraEvents, eventCompact)
			}
		}
		return extraEvents
	}

	if mode != "" {
		a, _ := dnslogbrokers.Get(params.Mode)
		if a != nil {
			results, err := a.GetResult(params.GetToken(), 15*time.Second)
			if err != nil {
				return nil, utils.Errorf("require[%v] dnslog failed: %s", a.Name(), err)
			}
			return &tpb.QueryExistedDNSLogResponse{
				Events: mergeResults(results),
			}, nil
		}
	}

	events, ok := D.cache.Get(params.GetToken())
	if !ok {
		return &tpb.QueryExistedDNSLogResponse{Events: mergeResults(nil)}, nil
	}
	rsp := &tpb.QueryExistedDNSLogResponse{Events: mergeResults(events)}
	return rsp, nil
}
func NewDNSLogServer(domain string, externalIP string) (*DNSLogGRPCServer, error) {
	return NewDNSLogServerWithListeningPort(domain, externalIP, 53)
}
func NewDNSLogServerWithListeningPort(domain string, externalIP string, port int) (*DNSLogGRPCServer, error) {
	ip := externalIP
	if externalIP == "" {
		ipIns, err := GetExternalIP()
		if err != nil {
			return nil, err
		}
		ip = ipIns.String()
	}

	if port <= 0 {
		port = 53
	}
	coreDNSServer, err := facades.NewDNSServer(domain, ip, "0.0.0.0", port)
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
	cache := utils.NewTTLCache[[]*tpb.DNSLogEvent](24 * time.Hour)
	cache.SetTTL(24 * time.Hour)
	tokenToModeCache := utils.NewTTLCache[string](24 * time.Hour)

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
			result, existed := cache.Get(token)
			if !existed {
				cache.Set(token, []*tpb.DNSLogEvent{event})
				return
			}
			result = append(result, event)
			cache.Set(token, result)
		} else {
		}
	})

	domains := utils.PrettifyListFromStringSplitEx(domain, ",", "|")
	grpcServe := &DNSLogGRPCServer{
		ExternalIP:       externalIP,
		domain:           domains,
		cache:            cache,
		tokenToModeCache: tokenToModeCache,
		core:             coreDNSServer,
	}
	return grpcServe, nil
}
