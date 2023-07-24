package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"time"
)

type digpm1433Broker struct {
}

func (c *digpm1433Broker) Name() string {
	return "dig.pm-1433"
}

var defaultDigPm1433 = &digpm1433Broker{}

func init() {
	register(defaultDigPm1433)
}

func (s *digpm1433Broker) GetResult(token string, du time.Duration, proxy ...string) ([]*tpb.DNSLogEvent, error) {
	var packet = `POST /get_results HTTP/1.1
Host: dig.pm
Accept-Encoding: identity
Content-type: application/x-www-form-urlencoded
Origin: https://dig.pm
Referer: https://dig.pm/
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36

domain=ipv6.1433.eu.org.&token=ab`

	rsp, err := lowhttp.HTTP(
		lowhttp.WithRequest(lowhttp.ReplaceHTTPPacketPostParam([]byte(packet), "token", token)),
		lowhttp.WithHttps(true),
		lowhttp.WithTimeout(du),
		lowhttp.WithProxy(proxy...),
	)
	if err != nil {
		return nil, err
	}

	/*
		{
		    "0": {
		        "ip": "172.253.0.3:63509",
		        "subdomain": "Bf3B80Fb.ipv6.1433.eu.org.",
		        "time": "2023-07-19T12:58:15+08:00"
		    }
		}
	*/
	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	var results []*tpb.DNSLogEvent
	for _, data := range utils.ParseStringToGeneralMap(body) {
		if data == nil {
			continue
		}

		params := utils.InterfaceToMapInterface(data)
		if params == nil {
			continue
		}
		subdomain := utils.MapGetString(params, "subdomain")
		if subdomain == "" {
			continue
		}

		var remoteAddr string
		var remoteIP string
		var remotePort int32
		if ret := utils.MapGetString(params, "ip"); ret != "" {
			remoteAddr = ret
			host, port, _ := utils.ParseStringToHostPort(remoteAddr)
			remoteIP = host
			remotePort = int32(port)
		}
		var ts int64
		t, err := time.Parse(time.RFC3339, utils.MapGetString(params, "time"))
		if err != nil {
			log.Errorf(`time.Parse(time.RFC3339, utils.MapGetString(params, "time")) err: %v`, err)
		}
		if !t.IsZero() {
			ts = t.Unix()
		}
		var raw = []byte(spew.Sdump(data))
		results = append(results, &tpb.DNSLogEvent{
			Type:       "A",
			Token:      token,
			Domain:     subdomain,
			RemoteAddr: remoteAddr,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			Raw:        raw,
			Timestamp:  ts,
			Mode:       s.Name(),
		})
	}
	if len(results) > 0 {
		return results, nil
	}

	return nil, utils.Error("emtpy result or not implemented")
}

func (s *digpm1433Broker) Require(du time.Duration, proxy ...string) (string, string, error) {
	packet := `POST /new_gen HTTP/1.1
Host: dig.pm
Content-Type: application/x-www-form-urlencoded
Origin: https://dig.pm
Referer: https://dig.pm/
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36

domain=ipv6.1433.eu.org.`
	/*

	 */
	rspIns, err := lowhttp.HTTP(
		lowhttp.WithHttps(true),
		lowhttp.WithRequest(packet),
		lowhttp.WithTimeout(du),
		lowhttp.WithProxy(proxy...),
	)
	if err != nil {
		return "", "", utils.Errorf("send dig.pm packet failed: %v", err)
	}
	rsp := rspIns.RawPacket
	_, body := lowhttp.SplitHTTPPacketFast(rspIns.RawPacket)
	var results = utils.ParseStringToGeneralMap(body)
	token := utils.MapGetString(results, "token")
	domain := utils.MapGetString(results, "domain")
	key := utils.MapGetString(results, "key")
	_ = key
	if token == "" || domain == "" {
		return "", "", utils.Errorf("cannot fetch token n domain from response: \n%v", string(rsp))
	}
	return domain, token, nil
}
