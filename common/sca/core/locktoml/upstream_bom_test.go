package locktoml

import (
	"context"
	"testing"
)

func TestUpstreamBOM(t *testing.T) {
	for _, prefix := range []string{"", "\ufeff"} {
		m, e := Parse(context.Background(), []byte(prefix+"version = 3\n"))
		if e != nil || m["version"] != int64(3) {
			t.Fatalf("UTF8 BOM: %v %v", m, e)
		}
	}
	// The upstream generic decoder stripped these invalid UTF-16 markers from
	// ASCII. Cargo/Poetry only admit valid UTF-8; do not silently reinterpret it.
	for _, prefix := range []string{"\xff\xfe", "\xfe\xff"} {
		if _, e := Parse(context.Background(), []byte(prefix+"version = 3\n")); e == nil {
			t.Fatal("accepted invalid UTF8 BOM")
		}
	}
}
