package fingerprint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
)

func TestYamlOpCode_ShouldNotUseVersionIndexAsVendorIndex(t *testing.T) {
	// Regression test: vendor_index is optional in many rules. When it's unset (0),
	// we must not accidentally overwrite vendor with the captured version.
	yamlRule := `- methods:
    - headers:
        - key: Server
          value:
            product: iis
            regexp: (?:Microsoft-)?IIS(?:/([0-9.]+))?
            version_index: 1`

	rules, err := parsers.ParseYamlRule(yamlRule)
	if err != nil {
		t.Fatal(err)
	}

	rsp := []byte("HTTP/1.1 200 OK\r\nServer: Microsoft-IIS/8.5\r\n\r\n")
	info, err := rule.Execute(func(string) (*rule.MatchResource, error) {
		return rule.NewHttpResource(rsp), nil
	}, rules[0])
	if err != nil {
		t.Fatal(err)
	}

	if !assert.NotNil(t, info) {
		return
	}
	info.Init()

	assert.Equal(t, "iis", info.Product)
	assert.Equal(t, "8.5", info.Version)
	assert.Equal(t, "*", info.Vendor)
}
