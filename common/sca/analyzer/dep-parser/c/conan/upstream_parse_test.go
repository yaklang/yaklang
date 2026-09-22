// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package conan_test

import (
	"sort"
	"strings"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/c/conan"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		inputFile string // Test input file
		wantLibs  []types.Library
		wantDeps  []types.Dependency
	}{
		{
			name:      "happy path",
			inputFile: "testdata/happy.lock",
			wantLibs: []types.Library{
				{
					ID:        "pkga/0.0.1",
					Name:      "pkga",
					Locations: []types.Location{{StartLine: 13, EndLine: 22}},
					Version:   "0.0.1",
				},
				{
					ID:        "pkgb/system",
					Name:      "pkgb",
					Locations: []types.Location{{StartLine: 23, EndLine: 29}},
					Version:   "system",
					Indirect:  true,
				},
				{
					ID:        "pkgc/0.1.1",
					Name:      "pkgc",
					Locations: []types.Location{{StartLine: 30, EndLine: 35}},
					Version:   "0.1.1",
				},
			},
			wantDeps: []types.Dependency{
				{
					ID: "pkga/0.0.1",
					DependsOn: []string{
						"pkgb/system",
					},
				},
			},
		},
		{
			name:      "happy path. lock file with revisions support",
			inputFile: "testdata/happy2.lock",
			wantLibs: []types.Library{
				{
					ID:        "openssl/3.0.3",
					Name:      "openssl",
					Locations: []types.Location{{StartLine: 12, EndLine: 22}},
					Version:   "3.0.3",
				},
				{
					ID:        "zlib/1.2.12",
					Name:      "zlib",
					Locations: []types.Location{{StartLine: 23, EndLine: 30}},
					Version:   "1.2.12",
					Indirect:  true,
				},
			},
			wantDeps: []types.Dependency{
				{
					ID: "openssl/3.0.3",
					DependsOn: []string{
						"zlib/1.2.12",
					},
				},
			},
		},
		{
			name:      "happy path. lock file without dependencies",
			inputFile: "testdata/empty.lock",
		},
		{
			name:      "sad path. wrong ref format",
			inputFile: "testdata/sad.lock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := fixtures.OpenReader(tt.inputFile)
			require.NoError(t, err)
			defer f.Close()

			gotLibs, gotDeps, err := conan.NewParser().Parse(nil, f)
			if tt.inputFile == "testdata/sad.lock" {
				if err == nil {
					t.Fatal("malformed reference was silently ignored")
				}
				return
			}
			require.NoError(t, err)

			ids := map[string]string{}
			for i := range gotLibs {
				v := &gotLibs[i]
				ids[v.ID] = v.Name + "/" + v.Version
				if v.Source == "" {
					t.Fatal("lost full Conan reference")
				}
				v.ID = ids[v.ID]
				v.Source = ""
			}
			for i := range gotDeps {
				gotDeps[i].ID = ids[gotDeps[i].ID]
				for j, ref := range gotDeps[i].DependsOn {
					if id, ok := ids[ref]; ok {
						gotDeps[i].DependsOn[j] = id
					}
				}
				gotDeps[i].Requirements = nil
			}

			sort.Slice(gotLibs, func(i, j int) bool {
				ret := strings.Compare(gotLibs[i].Name, gotLibs[j].Name)
				if ret != 0 {
					return ret < 0
				}
				return gotLibs[i].Version < gotLibs[j].Version
			})

			assert.Equal(t, tt.wantLibs, gotLibs)
			assert.Equal(t, tt.wantDeps, gotDeps)
		})
	}
}
