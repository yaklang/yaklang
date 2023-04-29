package fp

import (
	"yaklang/common/utils"
	"testing"
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
