package yso

import (
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"testing"
)

func TestExportFunc(t *testing.T) {
	gadgetMatcher := regexp.MustCompile(`Get\w+JavaObject`)
	classesMatcher := regexp.MustCompile(`Generate\w+ClassObject`)
	filterName := []string{"GetSimplePrincipalCollectionJavaObject"}
	for name, f := range Exports {
		if utils.StringArrayContains(filterName, name) {
			continue
		}
		if gadgetMatcher.MatchString(name) {
			if v, ok := f.(func(options ...GenClassOptionFun) (*JavaObject, error)); ok {
				serIns, err := v(SetRuntimeExecEvilClass("whoami"))
				if err != nil {
					t.Fatal(err)
				}
				_, err = ToBytes(serIns)
				if err != nil {
					t.Fatal(err)
				}
			} else if v, ok := f.(func(cmd string) (*JavaObject, error)); ok {
				serIns, err := v("whoami")
				if err != nil {
					t.Fatal(err)
				}
				_, err = ToBytes(serIns)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				t.Fatal(utils.Errorf("unknown gadget func: %s", name))
			}
		} else if classesMatcher.MatchString(name) {

		}
	}
}
