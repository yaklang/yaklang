package dnslogbrokers

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var brokers = new(sync.Map)
var list []string
var registerMutex = new(sync.Mutex)

func register(i DNSLogBroker) {
	registerMutex.Lock()
	defer registerMutex.Unlock()

	var name = i.Name()
	_, ok := brokers.Load(name)
	if ok {
		log.Errorf("broker: %v is existed", name)
		return
	}
	brokers.Store(name, i)
	list = append(list, name)
}

func Random() string {
	registerMutex.Lock()
	defer registerMutex.Unlock()
	if len(list) == 0 {
		return ""
	}
	if strings.Join(list, "") == "*" {
		return ""
	}
	return list[rand.Intn(len(list))]
}

func BrokerNames() []string {
	var names []string
	brokers.Range(func(key, value interface{}) bool {
		names = append(names, key.(string))
		return true
	})
	return names
}

func AvialableBrokers() []string {
	var a = []string{"*"}
	brokers.Range(func(key, value any) bool {
		if ret := utils.InterfaceToString(key); ret != "" {
			a = append(a, ret)
		}
		return true
	})
	return a
}

func Get(name string) (DNSLogBroker, error) {
	raw, ok := brokers.Load(name)
	if !ok {
		return nil, utils.Errorf("dnsbroker [%v] no existed", name)
	}
	ins, ok := raw.(DNSLogBroker)
	if !ok {
		spew.Dump(raw)
		return nil, utils.Errorf("BUG: dnsbroker internal error, not DNSLogBroker Interface")
	}
	return ins, nil
}

type DNSLogBroker interface {
	Require(timeout time.Duration, proxy ...string) (domain, token string, err error)
	GetResult(token string, timeout time.Duration, proxy ...string) ([]*tpb.DNSLogEvent, error)
	Name() string
}

func GetDNSLogBroker(mode string) DNSLogBroker {
	switch mode {
	case defaultDNSLogCN.Name():
		return defaultDNSLogCN
	case defaultDigPMBYPASS.Name():
		return defaultDigPMBYPASS
	case defaultDigPm1433.Name():
		return defaultDigPm1433
	default:
		return nil
	}
}
