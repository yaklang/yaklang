package yso

import (
	"regexp"
	"testing"
)

func TestExportFunc(t *testing.T) {
	gadgetMatcher := regexp.MustCompile(`Get\w+JavaObject`)
	for name, f := range Exports {
		if gadgetMatcher.MatchString(name) {
			_ = f
		}
	}
}
