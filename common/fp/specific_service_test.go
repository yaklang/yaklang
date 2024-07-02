package fp

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"testing"
)

func TestFingerprintRule(t *testing.T) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := webfingerprint.ParseWebFingerprintRules(resp)
	rules, _ := GetDefaultWebFingerprintRules()

	config := NewConfig(WithWebFingerprintRule(rules), WithOnlyEnableWebFingerprint(true))
	matcher, err := NewFingerprintMatcher(nil, config)
	if err != nil {
		t.FailNow()
	}

	host, port := webfingerprint.MockWebFingerPrintByName("oracle_commerce,outlook_web_app")

	result, err := matcher.Match(host, port)

	if err != nil {
		t.FailNow()
	}
	spew.Dump(result.GetServiceName())
	spew.Dump(len(result.GetCPEs()))
}

func TestMUSTPASS_FingerprintRule(t *testing.T) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := webfingerprint.ParseWebFingerprintRules(resp)

	config := NewConfig(
		WithOnlyEnableWebFingerprint(true),
		WithFingerprintDataSize(204800),
	)
	matcher, err := NewFingerprintMatcher(nil, config)
	if err != nil {
		t.FailNow()
	}

	wantRules, host, port := webfingerprint.MockRandomWebFingerPrints()

	result, err := matcher.Match(host, port)

	if err != nil {
		t.FailNow()
	}
	spew.Dump(len(wantRules))
	//spew.Dump(result.GetServiceName())
	//spew.Dump(result.GetServiceName())
	resultMap := make(map[string]bool)
	for _, cpe := range result.Fingerprint.HttpFlows[0].CPEs {
		resultMap[cpe.Product] = true
	}

	for _, want := range wantRules {
		if want == "" {
			continue
		}
		if _, exists := resultMap[want]; !exists {
			t.Errorf("Expected product [%s] not found in the actual results", want)
		}
	}
}
