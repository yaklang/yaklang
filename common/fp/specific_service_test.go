package fp

import (
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"strings"
	"testing"
)

func TestMUSTPASS_FingerprintRule(t *testing.T) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := webfingerprint.ParseWebFingerprintRules(resp)

	rules, _ := webfingerprint.LoadDefaultDataSource()

	config := NewConfig(WithWebFingerprintRule(rules))
	matcher, err := NewFingerprintMatcher(nil, config)
	if err != nil {
		t.FailNow()
	}

	wantRules, host, port := webfingerprint.MockRandomWebFingerPrints()

	result, err := matcher.Match(host, port)

	if err != nil {
		t.FailNow()
	}
	//spew.Dump(wantRules)
	resultMap := make(map[string]bool)
	for _, cpe := range result.Fingerprint.CPEs {
		productName := strings.Split(cpe, ":")[3]
		//fmt.Println(productName)
		resultMap[productName] = true
	}

	for _, want := range wantRules {
		if _, found := resultMap[want]; !found {
			t.Errorf("Expected product [%s] not found in the actual results", want)
		}
	}
}
