package pprofutils

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/google/pprof/profile"
	"testing"
)

//go:embed cpu-pprof.prof
var cpuProf []byte

//go:embed mem-pprof.prof
var memProf []byte

func TestAutoAnalyze(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"CPU Profile", cpuProf},
		{"Memory Profile", memProf},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prof, err := profile.Parse(bytes.NewBuffer(tt.data))
			if err != nil {
				t.Fatal(err)
			}

			stats := analyzePprof(prof)
			if len(stats) == 0 {
				t.Fatal("no stats found")
			}

			if len(stats) < 10 {
				t.Fatal("stats count should be greater than 10")
			}

			var hasGreaterThanOne bool
			for _, stat := range stats {
				if stat.Value <= 0 {
					t.Fatal("invalid stat value")
				}

				if tt.name == "CPU Profile" {
					if stat.Percent > 1.0 {
						hasGreaterThanOne = true
					}
				} else {
					// Memory Profile
					if stat.Percent > 1.0 {
						t.Fatal("memory profile percent should not greater than 1.0")
					}
				}

				if stat.Percent <= 0 {
					t.Fatal("invalid stat percent: should greater than 0")
				}

				fmt.Println(stat.Dump())
			}

			if tt.name == "CPU Profile" && !hasGreaterThanOne {
				t.Fatal("cpu profile should have at least one stat greater than 1.0")
			}
		})
	}
}
