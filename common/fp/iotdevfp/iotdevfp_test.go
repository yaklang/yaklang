package iotdevfp

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestIotDevRule_Match(t *testing.T) {
	var result = false
	for _, r := range append(IoTDeviceRules, ApplicationDeviceRules...) {
		match, err := r.Match([]byte(`Powered by <a href="https://www.siteserver`))
		if err != nil {
			continue
		}
		spew.Dump(match)
		result = true
	}

	if !result {
		t.FailNow()
	}
}
