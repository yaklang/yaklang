package fp

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
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
	maxRetries := 5
	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		t.Logf("Attempt %d/%d", attempt, maxRetries)

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

		// 检查是否所有期望的产品都被找到
		var missingProducts []string
		for _, want := range wantRules {
			if want == "" {
				continue
			}
			if _, exists := resultMap[want]; !exists {
				missingProducts = append(missingProducts, want)
			}
		}

		// 如果没有缺失的产品，测试成功
		if len(missingProducts) == 0 {
			t.Logf("Test passed on attempt %d", attempt)
			return
		}

		// 记录错误，准备重试
		lastError = fmt.Errorf("Attempt %d failed: Missing products: %v", attempt, missingProducts)
		t.Logf("Attempt %d failed: Missing products: %v", attempt, missingProducts)
		if attempt < maxRetries {
			t.Logf("Retrying... (attempt %d failed)", attempt)
		}
	}

	// 所有重试都失败了
	t.Fatalf("Test failed after %d attempts. Last error: %v", maxRetries, lastError)
}
