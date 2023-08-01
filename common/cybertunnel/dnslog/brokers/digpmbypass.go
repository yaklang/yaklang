package dnslogbrokers

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"time"
)

type digpmByPassBroker struct {
	subDomain string
}

func (c *digpmByPassBroker) Name() string {
	return "dig.pm-bypass"
}

var defaultDigPMBYPASS = &digpmByPassBroker{}

func init() {
	register(defaultDigPMBYPASS)
}

func (s *digpmByPassBroker) GetResult(token string, du time.Duration, proxy ...string) ([]*tpb.DNSLogEvent, error) {
	var packet = `POST /get_results HTTP/1.1
Host: dig.pm
Content-Type: application/x-www-form-urlencoded
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36
Content-Length: 71

mainDomain=ipv6.bypass.eu.org.&token=ab&subDomain=cd`
	packet = string(lowhttp.ReplaceHTTPPacketPostParam([]byte(packet), "token", token))
	packet = string(lowhttp.ReplaceHTTPPacketPostParam([]byte(packet), "subDomain", s.subDomain))
	rsp, err := lowhttp.HTTP(
		lowhttp.WithRequest([]byte(packet)),
		lowhttp.WithHttps(true),
		lowhttp.WithTimeout(du),
		lowhttp.WithProxy(proxy...),
	)
	if err != nil {
		return nil, err
	}

	/*
		[
			{
				"UUID":"67860a48-d206-4f29-8139-7aaa624b8cb0",
				"ClientIp":"61.188.7.194",
				"FullDomain":"1111.9de657937f.ipv6.1433.eu.org.",
				"MainDomain":"ipv6.1433.eu.org.",
				"SubDomain":"9de657937f",
				"CreatedAt":"2023-08-01T07:37:11.821849Z",
				"UpdatedAt":"2023-08-01T07:37:11.821849Z"
			}
		]
	*/
	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	var events []*tpb.DNSLogEvent
	for k, data := range utils.ParseStringToGeneralMap(body) {
		if k == "err" {
			log.Errorf("digpm1433 error: %s", data)
			return nil, utils.Error(data)
		}
		break
	}
	var records []map[string]interface{}
	err = json.Unmarshal(body, &records)
	if err != nil {
		log.Fatalf("JSON Unmarshalling failed: %s", err)
	}

	for _, record := range records {
		fullDomain := utils.MapGetString(record, "FullDomain")
		if fullDomain == "" {
			continue
		}

		var remoteAddr string
		var remoteIP string
		var remotePort int32
		if ret := utils.MapGetString(record, "ClientIp"); ret != "" {
			remoteAddr = ret
			host, port, _ := utils.ParseStringToHostPort(remoteAddr)
			remoteIP = host
			remotePort = int32(port)
		}
		var ts int64
		t, err := time.Parse(time.RFC3339, utils.MapGetString(record, "CreatedAt"))
		if err != nil {
			log.Errorf(`time.Parse(time.RFC3339, utils.MapGetString(params, "time")) err: %v`, err)
		}
		if !t.IsZero() {
			ts = t.Unix()
		}
		var raw = []byte(spew.Sdump(record))
		events = append(events, &tpb.DNSLogEvent{
			Type:       "A",
			Token:      token,
			Domain:     fullDomain,
			RemoteAddr: remoteAddr,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
			Raw:        raw,
			Timestamp:  ts,
			Mode:       s.Name(),
		})
	}

	return events, nil
}

func (s *digpmByPassBroker) Require(du time.Duration, proxy ...string) (string, string, error) {
	packet := `POST /get_sub_domain HTTP/1.1
Content-Type: application/x-www-form-urlencoded
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36
Host: dig.pm
Content-Length: 28

mainDomain=ipv6.bypass.eu.org.`

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
	_, body := lowhttp.SplitHTTPPacketFast(rsp)
	var results = utils.ParseStringToGeneralMap(body)
	token := utils.MapGetString(results, "token")
	domain := utils.MapGetString(results, "fullDomain")
	if token == "" || domain == "" {
		return "", "", utils.Errorf("cannot fetch token n domain from response: \n%v", string(rsp))
	}
	s.subDomain = utils.MapGetString(results, "subDomain")
	return domain, token, nil
}
