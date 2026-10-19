package fp

import (
	"testing"

	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/utils"
)

func TestGetDefaultNmapServiceProbeRules(t *testing.T) {
	results, err := GetDefaultNmapServiceProbeRules()
	if err != nil {
		t.Errorf("parse nmap rules failed: %s", err)
		t.FailNow()
	}

	if len(results) <= 0 {
		t.Error("empty results is not allowed")
		t.FailNow()
	}
}

func TestGetDefaultWebFingerprintRules(t *testing.T) {
	results, err := GetDefaultWebFingerprintRules()
	if err != nil {
		t.Errorf("parse wapplayzer rules failed: %s", err)
		t.FailNow()
	}

	if len(results) <= 0 {
		t.Error("empty results is not allowed")
		t.FailNow()
	}
	products := map[string]bool{}
	var walk func(rule *rule.FingerPrintRule)
	walk = func(rule *rule.FingerPrintRule) {
		if rule == nil || rule.MatchParam == nil {
			return
		}
		if rule.MatchParam.Info != nil && rule.MatchParam.Info.Product != "" {
			products[rule.MatchParam.Info.Product] = true
		}
		for _, sub := range rule.MatchParam.SubRules {
			walk(sub)
		}
	}
	for _, item := range results {
		walk(item)
	}
	for _, product := range []string{"supervisord", "MiniCMS"} {
		if !products[product] {
			t.Errorf("stored web fingerprint product %s is not loaded", product)
		}
	}
}

func TestNmapProbeLibraryIncludesCurrentProbes(t *testing.T) {
	results, err := GetDefaultNmapServiceProbeRules()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for probe := range results {
		if probe != nil {
			names[probe.Name] = true
		}
	}
	for _, name := range []string{"MikroTik_Winbox", "MikroTik_Winbox_Legacy", "SAP-HDB-Init"} {
		if !names[name] {
			t.Errorf("current nmap probe %s is not loaded", name)
		}
	}
}

func TestGetDefaultNmapServiceProbeRules_RDP(t *testing.T) {
	results, err := GetDefaultNmapServiceProbeRules()
	if err != nil {
		t.Errorf("get default nmap service failed: %s", err)
		t.FailNow()
	}

	flag := false
	for probe, _ := range results {
		if utils.IntArrayContains(probe.DefaultPorts, 3389) &&
			probe.Name == "RdpSSLHybridAndHybridEx" {
			flag = true
		}
	}

	if !flag {
		t.Error("parse rdp.txt failed")
		t.FailNow()
	}
}
