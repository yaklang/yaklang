package vulinbox

import (
	"context"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"sync"
)

type matcher struct {
	surirule []*rule.Rule
	lock     sync.RWMutex
	cancel   context.CancelFunc
	ctx      context.Context
	callback func([]byte)
}

func (m *matcher) RunSingle() {
	if m.ctx != nil {
		select {
		case <-m.ctx.Done():
			return
		default:
		}
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.run()
}

func (m *matcher) run() {
	handler, err := pcaputil.GetPublicInternetPcapHandler()
	if err != nil {
		log.Error(err)
	}
	source := gopacket.NewPacketSource(handler, handler.LinkType())
	for {
		select {
		case <-m.ctx.Done():
			return
		case packet, ok := <-source.Packets():
			if !ok {
				return
			}
			if m.Match(packet.Data()) {
				if m.callback != nil {
					m.callback(packet.Data())
				}
			}
		}
	}
}

func (m *matcher) SetCallback(f func([]byte)) {
	m.callback = f
}
func (m *matcher) Match(data []byte) bool {
	for _, r := range m.surirule {
		if match.New(r).Match(data) {
			return true
		}
	}
	return false
}

func (m *matcher) AddRule(rules ...*rule.Rule) {
	m.lock.Lock()
	defer m.lock.Unlock()
RULELOOP:
	for _, r := range rules {
		for _, r2 := range m.surirule {
			if r.Raw == r2.Raw {
				continue RULELOOP
			}
		}
		m.surirule = append(m.surirule, r)
	}
}

func (m *matcher) RemoveRule(rules ...*rule.Rule) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i := 0; i < len(m.surirule); i++ {
		for _, r := range rules {
			if r.Raw == m.surirule[i].Raw {
				if i == len(m.surirule)-1 {
					m.surirule = m.surirule[:len(m.surirule)-1]
				} else {
					m.surirule[i] = m.surirule[len(m.surirule)-1]
					m.surirule = m.surirule[:len(m.surirule)-1]
				}
				break
			}
		}
	}
}
