package generate

import (
	"errors"
	"github.com/yaklang/yaklang/common/suricata/data/protocol"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type ModifierGenerator interface {
	Gen() []byte
}

type Generator interface {
	Gen() []byte
}

func New(r *rule.Rule) (Generator, error) {
	switch r.Protocol {
	case protocol.HTTP:
		return newHTTPGen(r)
	case protocol.TCP:
		return newTCPGen(r)
	}
	return nil, errors.New("not support protocol")
}
