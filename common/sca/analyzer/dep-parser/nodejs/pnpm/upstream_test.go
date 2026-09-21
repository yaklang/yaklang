// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package pnpm

import (
	"os"
	"sort"
	"strings"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		file     string // Test input file
		want     []types.Library
		wantDeps []types.Dependency
	}{
		{
			name:     "normal",
			file:     "testdata/pnpm-lock_normal.yaml",
			want:     pnpmNormal,
			wantDeps: pnpmNormalDeps,
		},
		{
			name:     "with dev deps",
			file:     "testdata/pnpm-lock_with_dev.yaml",
			want:     pnpmWithDev,
			wantDeps: pnpmWithDevDeps,
		},
		{
			name:     "many",
			file:     "testdata/pnpm-lock_many.yaml",
			want:     pnpmMany,
			wantDeps: pnpmManyDeps,
		},
		{
			name:     "archives",
			file:     "testdata/pnpm-lock_archives.yaml",
			want:     pnpmArchives,
			wantDeps: pnpmArchivesDeps,
		},
		{
			name:     "v6",
			file:     "testdata/pnpm-lock_v6.yaml",
			want:     pnpmV6,
			wantDeps: pnpmV6Deps,
		},
		{
			name:     "v6 with dev deps",
			file:     "testdata/pnpm-lock_v6_with_dev.yaml",
			want:     pnpmV6WithDev,
			wantDeps: pnpmV6WithDevDeps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			defer f.Close()

			got, deps, err := NewParser().Parse(nil, f)
			require.NoError(t, err)

			// Original component/dependency fields still match the pinned oracle;
			// native peer IDs are asserted separately, then projected for comparison.
			ids := map[string]string{}
			for i := range got {
				ids[got[i].ID] = got[i].Name + "@" + got[i].Version
				if got[i].Variant != got[i].ID {
					t.Fatal("lost native peer context")
				}
				got[i].ID = ids[got[i].ID]
				got[i].Variant = ""
				got[i].Source = ""
				got[i].Verification = ""
				got[i].DeclaredIntegrity = ""
				got[i].DeclaredVersion = ""
				got[i].Diagnostics = nil
			}
			for i := range deps {
				deps[i].ID = ids[deps[i].ID]
				deps[i].Requirements = nil
				for j, ref := range deps[i].DependsOn {
					if id, ok := ids[ref]; ok {
						deps[i].DependsOn[j] = id
					}
				}
			}

			sortLibs(got)
			sortLibs(tt.want)

			assert.Equal(t, tt.want, got)
			if tt.wantDeps != nil {
				sortDeps(deps)
				sortDeps(tt.wantDeps)
				assert.Equal(t, tt.wantDeps, deps)
			}
		})
	}
}

func sortDeps(deps []types.Dependency) {
	sort.Slice(deps, func(i, j int) bool {
		return strings.Compare(deps[i].ID, deps[j].ID) < 0
	})

	for i := range deps {
		sort.Strings(deps[i].DependsOn)
	}
}

func sortLibs(libs []types.Library) {
	sort.Slice(libs, func(i, j int) bool {
		ret := strings.Compare(libs[i].Name, libs[j].Name)
		if ret == 0 {
			return libs[i].Version < libs[j].Version
		}
		return ret < 0
	})
}

func Test_parsePackage(t *testing.T) {
	tests := []struct {
		name        string
		lockFileVer int
		pkg         string
		wantName    string
		wantVersion string
	}{
		{
			name:        "v5 - relative path",
			lockFileVer: 5,
			pkg:         "/lodash/4.17.10",
			wantName:    "lodash",
			wantVersion: "4.17.10",
		},
		{
			name:        "v5 - registry",
			lockFileVer: 5,
			pkg:         "registry.npmjs.org/lodash/4.17.10",
			wantName:    "lodash",
			wantVersion: "4.17.10",
		},
		{
			name:        "v5 - relative path with slash",
			lockFileVer: 5,
			pkg:         "/@babel/generator/7.21.9",
			wantName:    "@babel/generator",
			wantVersion: "7.21.9",
		},
		{
			name:        "v5 - registry path with slash",
			lockFileVer: 5,
			pkg:         "registry.npmjs.org/@babel/generator/7.21.9",
			wantName:    "@babel/generator",
			wantVersion: "7.21.9",
		},
		{
			name:        "v5 - relative path with slash and peer deps",
			lockFileVer: 5,
			pkg:         "/@babel/helper-compilation-targets/7.21.5_@babel+core@7.21.8",
			wantName:    "@babel/helper-compilation-targets",
			wantVersion: "7.21.5",
		},
		{
			name:        "v5 - relative path with underline and peer deps",
			lockFileVer: 5,
			pkg:         "/lodash._baseclone/4.5.7_@babel+core@7.21.8",
			wantName:    "lodash._baseclone",
			wantVersion: "4.5.7",
		},
		{
			name:        "v5 - registry with slash and peer deps",
			lockFileVer: 5,
			pkg:         "registry.npmjs.org/@babel/helper-compilation-targets/7.21.5_@babel+core@7.21.8",
			wantName:    "@babel/helper-compilation-targets",
			wantVersion: "7.21.5",
		},
		{
			name:        "v5 - relative path with wrong version",
			lockFileVer: 5,
			pkg:         "/lodash/4-wrong",
			wantName:    "",
			wantVersion: "",
		},
		{
			name:        "v6 - relative path",
			lockFileVer: 6,
			pkg:         "/update-browserslist-db@1.0.11",
			wantName:    "update-browserslist-db",
			wantVersion: "1.0.11",
		},
		{
			name:        "v6 - registry",
			lockFileVer: 6,
			pkg:         "registry.npmjs.org/lodash@4.17.10",
			wantName:    "lodash",
			wantVersion: "4.17.10",
		},
		{
			name:        "v6 - relative path with slash",
			lockFileVer: 6,
			pkg:         "/@babel/helper-annotate-as-pure@7.18.6",
			wantName:    "@babel/helper-annotate-as-pure",
			wantVersion: "7.18.6",
		},
		{
			name:        "v6 - registry with slash",
			lockFileVer: 6,
			pkg:         "registry.npmjs.org/@babel/helper-annotate-as-pure@7.18.6",
			wantName:    "@babel/helper-annotate-as-pure",
			wantVersion: "7.18.6",
		},
		{
			name:        "v6 - relative path with slash and peer deps",
			lockFileVer: 6,
			pkg:         "/@babel/helper-compilation-targets@7.21.5(@babel/core@7.20.7)",
			wantName:    "@babel/helper-compilation-targets",
			wantVersion: "7.21.5",
		},
		{
			name:        "v6 - registry with slash and peer deps",
			lockFileVer: 6,
			pkg:         "registry.npmjs.org/@babel/helper-compilation-targets@7.21.5(@babel/core@7.20.7)",
			wantName:    "@babel/helper-compilation-targets",
			wantVersion: "7.21.5",
		},
		{
			name:        "v6 - relative path with wrong version",
			lockFileVer: 6,
			pkg:         "/lodash@4-wrong",
			wantName:    "",
			wantVersion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVersion := parsePackage(tt.pkg, tt.lockFileVer)
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantVersion, gotVersion)
		})

	}
}
