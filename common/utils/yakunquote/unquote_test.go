package yakunquote

import (
	"fmt"
	"testing"
)

func TestUnquoteInvalidUTF8(t *testing.T) {
	raw := "你好"
	hexStr := ""

	for _, r := range []byte(raw) {
		hexStr += fmt.Sprintf("\\x%x", r)
	}
	got, err := Unquote(fmt.Sprintf(`"%s"`, hexStr))
	if err != nil {
		t.Fatal(err)
	}
	if got != raw {
		t.Fatalf("want %s, got %s", raw, got)
	}
}
