package netx

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
	"net/url"
)

/*
DoH is a DNS over HTTPS resolver

It is used to resolve a domain name to an IP address via http(s) / tls
default situation:

1. http://1.1.1.1/dns-query to resolve domain name to ip address
2. https://dns.google/dns-query to resolve domain name to ip address
3. https://cloudflare-dns.com/dns-query to resolve domain name to ip address
4. https://dns.alidns.com/dns-query to resolve domain name to ip address

actually http://ip/dns-query have json api.
but... only few api source can be used.
*/

type DoHDNSResponse struct {
	Status   int  `json:"Status"`
	TC       bool `json:"TC"`
	RD       bool `json:"RD"`
	RA       bool `json:"RA"`
	AD       bool `json:"AD"`
	CD       bool `json:"CD"`
	Question json.RawMessage
	Answer   []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func dohRequest(domain string, dohUrl string, config *ReliableDNSConfig) error {
	log.Debugf("start to request doh: %v to %v", domain, dohUrl)
	var val = make(url.Values)
	val.Set("name", domain)
	val.Set("type", "1")

	var baseCtx context.Context
	if config.BaseContext != nil {
		baseCtx = config.BaseContext
	} else {
		baseCtx = context.Background()
	}

	ctx, cancel := context.WithTimeout(baseCtx, config.Timeout)
	defer cancel()
	reqInstance, err := http.NewRequestWithContext(ctx, "GET", dohUrl, nil)
	if err != nil {
		return utils.Errorf("build doh request failed: %s", err)
	}
	reqInstance.URL.RawQuery = val.Encode()
	reqInstance.Header.Set("Accept", "application/dns-json")
	rspInstance, err := NewDefaultHTTPClient().Do(reqInstance)
	if err != nil {
		return utils.Errorf("doh request failed: %s", err)
	}
	body, _ := io.ReadAll(rspInstance.Body)
	log.Infof("doh[%v-%v] fetch: %v", domain, dohUrl, string(body))
	var rspObj DoHDNSResponse
	err = json.Unmarshal(body, &rspObj)
	if err != nil {
		println(string(body))
		log.Errorf("unmarshal doh response failed: %s", err)
	}
	for _, a := range rspObj.Answer {
		config.call("A", domain, a.Data, dohUrl, "yakdns.doh", a.TTL)
	}
	return nil
}
