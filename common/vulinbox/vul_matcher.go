package vulinbox

import (
	"context"
	"github.com/google/gopacket"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"golang.org/x/exp/slices"
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
		default:
			return
		}
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.run()
}

func (m *matcher) run() {
	handler, err := pcaputil.GetPublicInternetPcapHandler()
	if err != nil {
		log.Error(err)
		return
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
	for _, r := range rules {
		var found bool
		for _, r2 := range m.surirule {
			if r.Raw == r2.Raw {
				found = true
				break
			}
		}
		if !found {
			m.surirule = append(m.surirule, r)
		}
	}
}

func (m *matcher) RemoveRule(rules ...*rule.Rule) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.surirule = slices.DeleteFunc(m.surirule, func(rr *rule.Rule) bool {
		return slices.IndexFunc(rules, func(r *rule.Rule) bool {
			return r.Raw == rr.Raw
		}) != -1
	})
	if len(m.surirule) == 0 {
		m.cancel()
	}
}
