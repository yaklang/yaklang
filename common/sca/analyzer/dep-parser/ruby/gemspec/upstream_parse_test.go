// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package gemspec_test

import (
	"os"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/gemspec"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name           string
		inputFile      string
		want           []types.Library
		wantErr        string
		wantDiagnostic string
	}{
		{
			name:      "happy",
			inputFile: "testdata/normal00.gemspec",
			want: []types.Library{{
				Name:        "rake",
				Version:     "13.0.3",
				License:     "MIT",
				RawLicenses: []string{"MIT"},
			}},
		},
		{
			name:           "another variable name",
			inputFile:      "testdata/normal01.gemspec",
			wantDiagnostic: "malformed_input",
			want: []types.Library{{
				Name:    "async",
				Version: "1.25.0",
			}},
		},
		{
			name:      "license",
			inputFile: "testdata/license.gemspec",
			want: []types.Library{{
				Name:        "async",
				Version:     "1.25.0",
				License:     "MIT",
				RawLicenses: []string{"MIT"},
			}},
		},
		{
			name:      "multiple licenses",
			inputFile: "testdata/multiple_licenses.gemspec",
			want: []types.Library{{
				Name:        "test-unit",
				Version:     "3.3.7",
				License:     "Ruby, BSDL, PSFL",
				RawLicenses: []string{"Ruby", "BSDL", "PSFL"},
			}},
		},
		{
			name:      "malformed variable name",
			inputFile: "testdata/malformed00.gemspec",
			wantErr:   "malformed_input",
		},
		{
			name:      "missing version",
			inputFile: "testdata/malformed01.gemspec",
			wantErr:   "malformed_input",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.inputFile)
			require.NoError(t, err)

			got, _, err := gemspec.NewParser().Parse(nil, f)
			if tt.wantErr != "" {
				require.NotNil(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantDiagnostic != "" {
				if len(got) != 1 || len(got[0].Diagnostics) != 1 || got[0].Diagnostics[0].Code != tt.wantDiagnostic || !got[0].Diagnostics[0].Incomplete {
					t.Fatalf("missing typed license diagnostic: %+v", got)
				}
			}
			for i := range got {
				got[i].ID = ""
				got[i].Source = ""
				got[i].Locations = nil
				got[i].Diagnostics = nil
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
