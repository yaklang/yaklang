package dnslogbrokers

import (
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"time"
)

type z9zTopBroker struct {
}

func (d *z9zTopBroker) Require(timeout time.Duration, proxy ...string) (domain, token string, err error) {
	var r string
	var samples, _ = mutate.FuzzTagExec(`{{randint(10000, 99999)}}{{randint(10000, 99999)}}{{randint(10000, 99999)}}{{randint(1000, 9999)}}`)
	if len(samples) > 0 {
		r = samples[0]
	}

	dns_lookup.LookupFirst(`dnslog.cn`, dns_lookup.WithTimeout(5*time.Second))
	packet := []byte(`GET /getdomain/?t=0.06596369931824886 HTTP/1.1
Host: dnslog.bczs.net
Accept: */*
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Referer: http://dnslog.bczs.net/
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36

`)
	packet = lowhttp.ReplaceHTTPPacketQueryParam(packet, "t", `0.`+r)
	rsp, err := lowhttp.HTTP(
		lowhttp.WithRequest(packet),
		lowhttp.WithTimeout(timeout),
		lowhttp.WithProxy(proxy...),
		lowhttp.WithDNSServers([]string{"1.1.1.1", "223.5.5.5"}),
	)
	if err != nil {
		return "", "", utils.Errorf("fetch dnslog.cn token failed: %s", err)
	}

	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	subdomain := string(body)
	token = lowhttp.GetHTTPPacketCookie(rsp.RawPacket, "sessionid")

	if token == "" || subdomain == "" {
		return "", "", utils.Errorf("lowhttp.GetHTTPPacketCookie failed: %v", "cookie or subdomain is empty")
	}
	log.Debugf("dnslog.bczs.net subdomain: %s, token: %s", subdomain, token)
	return subdomain, token, nil
}

func (d *z9zTopBroker) GetResult(token string, timeout time.Duration, proxy ...string) ([]*tpb.DNSLogEvent, error) {
	log.Debugf("dnslog.bczs.net token: %s", token)
	var r string
	var samples, _ = mutate.FuzzTagExec(`{{randint(10000, 99999)}}{{randint(10000, 99999)}}{{randint(10000, 99999)}}{{randint(1000, 9999)}}`)
	if len(samples) > 0 {
		r = samples[0]
	}

	packet := []byte(`GET /getrecords/?t=0.38722448860909564 HTTP/1.1
Host: dnslog.bczs.net
Accept: */*
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cookie: PHPSESSID=aaa
Referer: http://dnslog.bczs.net/
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36

`)
	packet = lowhttp.ReplaceHTTPPacketQueryParam(packet, "t", `0.`+r)
	packet = lowhttp.ReplaceHTTPPacketCookie(packet, "sessionid", token)
	rspIns, err := lowhttp.HTTP(
		lowhttp.WithRequest(packet),
		lowhttp.WithTimeout(timeout),
		lowhttp.WithProxy(proxy...),
		lowhttp.WithDNSServers([]string{"1.1.1.1", "223.5.5.5"}),
	)
	if err != nil {
		log.Errorf("lowhttp.HTTP failed: %s", err)
		return nil, err
	}
	var i interface{}
	_, body := lowhttp.SplitHTTPPacketFast(rspIns.RawPacket)
	if len(body) > 0 {
		json.Unmarshal(body, &i)
	}
	if i == nil {
		return nil, nil
	}
	var events []*tpb.DNSLogEvent
	funk.ForEach(i, func(sub any) {
		params := utils.InterfaceToMapInterface(sub)
		if params == nil {
			return
		}
		var subdomain = params["0"].(string)
		if subdomain == "" {
			return
		}
		var timeStr = params["2"].(string)

		var event = &tpb.DNSLogEvent{
			Type:       params["1"].(string),
			Token:      token,
			Domain:     subdomain,
			RemoteAddr: "",
			RemoteIP:   "",
			Raw:        []byte(spew.Sdump(sub)),
			Mode:       d.Name(),
		}

		var ts int64
		t, _ := time.Parse(time.DateTime, timeStr)
		if !t.IsZero() {
			ts = t.Unix()
			event.Timestamp = ts
		}
		events = append(events, event)
	})
	return events, nil
}

func (d *z9zTopBroker) Name() string {
	return "z9z.top"
}

var defaultZ9zTop = &z9zTopBroker{}

func init() {
	register(defaultZ9zTop)
}
