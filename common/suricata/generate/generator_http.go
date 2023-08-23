package generate

import (
	"bytes"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

type HeaderGen []*ContentGen

func parse2HeaderGen(rules []*rule.ContentRule) *HeaderGen {
	ctg := parse2ContentGen(rules)
	var hdg HeaderGen
	tmp := new(ContentGen)
	tmp.noise = noiseChar
	for _, mdf := range ctg.Modifiers {
		switch mdf := mdf.(type) {
		case *ContentModifier:
			if !mdf.Relative && len(tmp.Modifiers) != 0 {
				tmp.setLen()
				hdg = append(hdg, tmp)
				tmp = new(ContentGen)
				tmp.noise = noiseChar
			}
		case *RegexpModifier:
			if !mdf.Generator.Relative() && len(tmp.Modifiers) != 0 {
				tmp.setLen()
				hdg = append(hdg, tmp)
				tmp = new(ContentGen)
				tmp.noise = noiseChar
			}
		}
		tmp.Modifiers = append(tmp.Modifiers, mdf)
	}
	if len(tmp.Modifiers) != 0 {
		tmp.setLen()
		hdg = append(hdg, tmp)
		tmp = new(ContentGen)
	}
	return &hdg
}

func (h *HeaderGen) Gen() []byte {
	var res []byte
	for _, gen := range *h {
		tmp := gen.Gen()
		if !bytes.HasSuffix(tmp, []byte(lowhttp.CRLF)) {
			tmp = append(tmp, []byte(lowhttp.CRLF)...)
		}
		res = append(res, tmp...)
	}
	return res
}
