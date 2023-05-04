package subdomain

import (
	"context"
	"testing"
)

func TestGenerateMainDictionary(t *testing.T) {
	count := 0
	for range GenerateMainDictionary(context.Background()) {
		count++
	}

	if count < 3000 {
		t.Logf("default main dict line is %v", count)
		t.FailNow()
	}
}

func TestGenerateSubDictionary(t *testing.T) {
	count := 0
	for range GenerateSubDictionary(context.Background()) {
		count++
	}

	if count < 163 {
		t.Logf("default sub dict line is %v", count)
		t.FailNow()
	}
}
