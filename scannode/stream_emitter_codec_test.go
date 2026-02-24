package scannode

import (
	"crypto/rand"
	"strings"
	"testing"
)

func TestMaybeCompressSnappy(t *testing.T) {
	e := &StreamEmitter{codec: "snappy"}

	tests := []struct {
		name        string
		makePayload func() []byte
		wantEnc     string
		wantSmaller bool
	}{
		{
			name:        "compressible",
			makePayload: func() []byte { return []byte(strings.Repeat("abc123xyz", 4096)) },
			wantEnc:     "snappy",
			wantSmaller: true,
		},
		{
			name: "incompressible_random",
			makePayload: func() []byte {
				raw := make([]byte, 32*1024)
				rand.Read(raw)
				return raw
			},
			wantEnc:     "",
			wantSmaller: false,
		},
		{
			name:        "too_small_skipped",
			makePayload: func() []byte { return []byte("short") },
			wantEnc:     "",
			wantSmaller: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.makePayload()
			out, enc := e.maybeCompress(raw)
			if enc != tt.wantEnc {
				t.Fatalf("encoding mismatch: got=%q want=%q", enc, tt.wantEnc)
			}
			if tt.wantSmaller && len(out) >= len(raw) {
				t.Fatalf("expected smaller output: compressed=%d raw=%d", len(out), len(raw))
			}
			if !tt.wantSmaller && len(out) != len(raw) {
				t.Fatalf("expected same size: got=%d want=%d", len(out), len(raw))
			}
		})
	}
}
