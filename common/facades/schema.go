package facades

import (
	"fmt"
	"time"
)

type VisitorLog struct {
	// dns
	Type string

	Details map[string]interface{}
}

func NewVisitorLog(t string) *VisitorLog {
	vlog := &VisitorLog{
		Type:    t,
		Details: make(map[string]interface{}),
	}
	return vlog
}

func (v *VisitorLog) Set(k string, val interface{}) {
	v.Details[k] = val
}

func (v *VisitorLog) SetRemoteIP(remoteAddr string) {
	v.Set("remote-addr", remoteAddr)
}

func (v *VisitorLog) SetDomain(domain string) {
	v.Set("domain", domain)
}

func (v *VisitorLog) GetDomain() string {
	i, ok := v.Details["domain"]
	if ok {
		return fmt.Sprint(i)
	}
	return ""
}

func (v *VisitorLog) SetTimestampNow() {
	v.Set("timestamp", time.Now().Unix())
}

func (v *VisitorLog) SetDNSType(dnsType string) {
	v.Set("dns-type", dnsType)
}

type FacadeCallback func(i *VisitorLog)
