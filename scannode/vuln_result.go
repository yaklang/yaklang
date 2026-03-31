package scannode

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/spec"
)

// NewVulnResult converts the local vuln model into the platform scan result.
func NewVulnResult(v *Vuln) (*spec.ScanResult, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &spec.ScanResult{
		Type:    spec.ScanResult_Vuln,
		Content: raw,
	}, nil
}
