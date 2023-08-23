package vulinbox

import (
	"github.com/yaklang/yaklang/common/suricata/rule"
	"sync"
)

type matcher struct {
	// todo: use sorted slice to pref
	rules []*rule.Rule
	lock  sync.RWMutex
}

func (m *matcher) Match(data []byte) bool {
	// todo: implement
	return true
}

func (m *matcher) AddRule(rules ...*rule.Rule) {
	// todo: skip if already exists
	m.lock.Lock()
	defer m.lock.Unlock()
	m.rules = append(m.rules, rules...)
}

func (m *matcher) RemoveRule(rules ...*rule.Rule) {
	m.lock.Lock()
	defer m.lock.Unlock()
	for i := 0; i < len(m.rules); i++ {
		for _, r := range rules {
			if r.Raw == m.rules[i].Raw {
				if i == len(m.rules)-1 {
					m.rules = m.rules[:len(m.rules)-1]
				} else {
					m.rules[i] = m.rules[len(m.rules)-1]
					m.rules = m.rules[:len(m.rules)-1]
				}
				break
			}
		}
	}
}
