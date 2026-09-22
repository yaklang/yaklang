package analyzer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/core/budget"
)

func TestZipDirectoryReservation(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"one", "two"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("data")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	raw := b.Bytes()
	for _, tc := range []struct {
		name   string
		limits budget.Limits
		want   string
	}{
		{"positive", budget.Limits{}, ""},
		{"entries", budget.Limits{MaxArchiveEntries: 1}, "resource_limit"},
		{"working", budget.Limits{MaxResultBytes: 64}, "resource_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := budget.Bind(context.Background(), tc.limits)
			err := reserveZipDirectory(ctx, bytes.NewReader(raw), int64(len(raw)))
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("got %v", err)
			}
		})
	}
	// Claiming one entry must not hide the second central record from the
	// shared entry budget. No archive/zip File allocation has happened yet.
	forged := bytes.Clone(raw)
	binary.LittleEndian.PutUint16(forged[len(forged)-12:], 1)
	err := reserveZipDirectory(budget.Bind(context.Background(), budget.Limits{MaxArchiveEntries: 1}), bytes.NewReader(forged), int64(len(forged)))
	if err == nil || !strings.Contains(err.Error(), "resource_limit") {
		t.Fatalf("forged count bypassed budget: %v", err)
	}
	err = reserveZipDirectory(budget.Bind(context.Background(), budget.Limits{}), bytes.NewReader(forged), int64(len(forged)))
	if err == nil || !strings.Contains(err.Error(), "malformed_input") {
		t.Fatalf("count mismatch accepted: %v", err)
	}
}
