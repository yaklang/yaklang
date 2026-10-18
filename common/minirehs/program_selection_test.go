package minirehs

import (
	"fmt"
	"strings"
	"testing"
)

func TestEngineGlobalFindAllSelection(t *testing.T) {
	for _, expr := range []string{`aa`, `a{2,3}`, `aa|aaa`, `(?:ab){1,3}`, `(?i)ss`, `\baa\b`} {
		for _, locate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/loc=%v", expr, locate), func(t *testing.T) {
				patterns := []Pattern{{ID: 17, Expr: expr}}
				db, err := Compile(patterns, WithBackend(BackendEngine), WithReportLocation(locate), WithLogger(silentLogger{}))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				for _, data := range []string{"", "aaaaaaaaaa", "xaaaaaaaaaax", strings.Repeat("ab", 40), "ſſſsss", " aa aa "} {
					for _, stop := range []int{0, 1, 2, 3} {
						got := programScan(t, db, []byte(data), stop)
						want := programOracle(patterns, []byte(data), BackendEngine, locate, stop)
						if err := programCompare(got, want, true); err != nil {
							t.Fatalf("data=%q stop=%d: %s", data, stop, err)
						}
					}
				}
			})
		}
	}
}
