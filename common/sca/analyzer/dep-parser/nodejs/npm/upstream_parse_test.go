// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package npm

import (
	"encoding/json"
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
			name:     "lock version v1",
			file:     "testdata/package-lock_v1.json",
			want:     npmV1Libs,
			wantDeps: npmDeps,
		},
		{
			name:     "lock version v2",
			file:     "testdata/package-lock_v2.json",
			want:     npmV2Libs,
			wantDeps: npmV2Deps,
		},
		{
			name:     "lock version v3",
			file:     "testdata/package-lock_v3.json",
			want:     npmV2Libs,
			wantDeps: npmV2Deps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.Open(tt.file)
			require.NoError(t, err)

			got, deps, err := NewParser().Parse(nil, f)
			require.NoError(t, err)

			// API projection only: preserve independent upstream component and
			// dependency assertions while checking physical IDs in instance_test.go.
			ids := map[string]string{}
			for _, v := range got {
				ids[v.ID] = v.Name + "@" + v.Version
			}
			for i := range got {
				v := &got[i]
				v.ID = ids[v.ID]
				v.ExternalReferences = []types.ExternalRef{{Type: types.RefOther, URL: v.Source}}
				v.Source = ""
				v.Verification = ""
				v.Diagnostics = nil
				v.Evidence = ""
			}
			for i := range deps {
				d := &deps[i]
				d.ID = ids[d.ID]
				for _, q := range d.Requirements {
					if q.Resolved != "" {
						d.DependsOn = append(d.DependsOn, ids[q.Resolved])
					}
				}
				d.Requirements = nil
			}
			nonempty := deps[:0]
			for _, d := range deps {
				if len(d.DependsOn) > 0 {
					nonempty = append(nonempty, d)
				}
			}
			deps = nonempty
			groups := map[string]types.Library{}
			for _, v := range got {
				old, ok := groups[v.ID]
				if !ok {
					groups[v.ID] = v
					continue
				}
				old.Locations = append(old.Locations, v.Locations...)
				old.Dev = old.Dev && v.Dev
				old.Indirect = old.Indirect && v.Indirect
				groups[v.ID] = old
			}
			got = nil
			for _, v := range groups {
				got = append(got, v)
			}
			unique := map[string]bool{}
			projected := deps[:0]
			for _, d := range deps {
				sort.Strings(d.DependsOn)
				raw, _ := json.Marshal(d)
				if !unique[string(raw)] {
					unique[string(raw)] = true
					projected = append(projected, d)
				}
			}
			deps = projected
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
	for _, lib := range libs {
		sortLocations(lib.Locations)
	}
	sort.Slice(libs, func(i, j int) bool {
		return strings.Compare(libs[i].ID, libs[j].ID) < 0
	})
}

func sortLocations(locs []types.Location) {
	sort.Slice(locs, func(i, j int) bool {
		return locs[i].StartLine < locs[j].StartLine
	})
}
