package fingerprint

import (
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
)

func LoadCPEFromWebfingerrintCPE(o *webfingerprint.CPE) *rule.CPE {
	return &rule.CPE{
		Part:     o.Part,
		Vendor:   o.Vendor,
		Product:  o.Product,
		Version:  o.Version,
		Update:   o.Update,
		Edition:  o.Edition,
		Language: o.Language,
	}
}
