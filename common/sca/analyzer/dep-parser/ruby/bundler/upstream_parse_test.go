// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package bundler_test

import (
	"fmt"
	"sort"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/ruby/bundler"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

var (
	NormalLibs = []types.Library{
		{
			ID:        "coderay@1.1.2",
			Name:      "coderay",
			Version:   "1.1.2",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 4, EndLine: 4}},
		},
		{
			ID:        "concurrent-ruby@1.1.5",
			Name:      "concurrent-ruby",
			Version:   "1.1.5",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 5, EndLine: 5}},
		},
		{
			ID:        "dotenv@2.7.2",
			Name:      "dotenv",
			Version:   "2.7.2",
			Locations: []types.Location{{StartLine: 6, EndLine: 6}},
		},
		{
			ID:        "faker@1.9.3",
			Name:      "faker",
			Version:   "1.9.3",
			Locations: []types.Location{{StartLine: 7, EndLine: 7}},
		},
		{
			ID:        "i18n@1.6.0",
			Name:      "i18n",
			Version:   "1.6.0",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 9, EndLine: 9}},
		},
		{
			ID:        "method_source@0.9.2",
			Name:      "method_source",
			Version:   "0.9.2",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 11, EndLine: 11}},
		},
		{
			ID:        "pry@0.12.2",
			Name:      "pry",
			Version:   "0.12.2",
			Locations: []types.Location{{StartLine: 12, EndLine: 12}},
		},
	}
	NormalDeps = []types.Dependency{
		{
			ID:        "faker@1.9.3",
			DependsOn: []string{"i18n@1.6.0"},
		},
		{
			ID:        "i18n@1.6.0",
			DependsOn: []string{"concurrent-ruby@1.1.5"},
		},
		{
			ID: "pry@0.12.2",
			DependsOn: []string{
				"coderay@1.1.2",
				"method_source@0.9.2",
			},
		},
	}
	Bundler2Libs = []types.Library{
		{
			ID:        "coderay@1.1.3",
			Name:      "coderay",
			Version:   "1.1.3",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 4, EndLine: 4}},
		},
		{
			ID:        "concurrent-ruby@1.1.10",
			Name:      "concurrent-ruby",
			Version:   "1.1.10",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 5, EndLine: 5}},
		},
		{
			ID:        "dotenv@2.7.6",
			Name:      "dotenv",
			Version:   "2.7.6",
			Locations: []types.Location{{StartLine: 6, EndLine: 6}},
		},
		{
			ID:        "faker@2.21.0",
			Name:      "faker",
			Version:   "2.21.0",
			Locations: []types.Location{{StartLine: 7, EndLine: 7}},
		},
		{
			ID:        "i18n@1.10.0",
			Name:      "i18n",
			Version:   "1.10.0",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 9, EndLine: 9}},
		},
		{
			ID:        "json@2.6.2",
			Name:      "json",
			Version:   "2.6.2",
			Locations: []types.Location{{StartLine: 11, EndLine: 11}},
		},
		{
			ID:        "method_source@1.0.0",
			Name:      "method_source",
			Version:   "1.0.0",
			Indirect:  true,
			Locations: []types.Location{{StartLine: 12, EndLine: 12}},
		},
		{
			ID:        "pry@0.14.1",
			Name:      "pry",
			Version:   "0.14.1",
			Locations: []types.Location{{StartLine: 13, EndLine: 13}},
		},
	}
	Bundler2Deps = []types.Dependency{
		{
			ID:        "faker@2.21.0",
			DependsOn: []string{"i18n@1.10.0"},
		},
		{
			ID:        "i18n@1.10.0",
			DependsOn: []string{"concurrent-ruby@1.1.10"},
		},
		{
			ID: "pry@0.14.1",
			DependsOn: []string{
				"coderay@1.1.3",
				"method_source@1.0.0",
			},
		},
	}
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantLibs []types.Library
		wantDeps []types.Dependency
		wantErr  assert.ErrorAssertionFunc
	}{
		{
			name:     "normal",
			file:     "testdata/Gemfile_normal.lock",
			wantLibs: NormalLibs,
			wantDeps: NormalDeps,
			wantErr:  assert.NoError,
		},
		{
			name:     "bundler2",
			file:     "testdata/Gemfile_bundler2.lock",
			wantLibs: Bundler2Libs,
			wantDeps: Bundler2Deps,
			wantErr:  assert.NoError,
		},
		{
			name:     "malformed",
			file:     "testdata/Gemfile_malformed.lock",
			wantLibs: []types.Library{},
			wantErr:  assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := fixtures.OpenReader(tt.file)
			require.NoError(t, err)
			defer f.Close()

			p := &bundler.Parser{}
			gotLibs, gotDeps, err := p.Parse(nil, f)
			if !tt.wantErr(t, err, fmt.Sprintf("Parse(%v)", tt.file)) {
				return
			}
			if err != nil {
				return
			}
			ids := map[string]string{}
			for i := range gotLibs {
				v := &gotLibs[i]
				ids[v.ID] = v.Name + "@" + v.Version
				v.ID = ids[v.ID]
				v.Source = ""
				v.Variant = ""
			}
			for i := range gotDeps {
				d := &gotDeps[i]
				d.ID = ids[d.ID]
				for _, r := range d.Requirements {
					if r.Resolved != "" {
						d.DependsOn = append(d.DependsOn, ids[r.Resolved])
					}
				}
				d.Requirements = nil
			}
			// Projection changes the opaque native IDs to the old name@version
			// keys. Compare in that key order, independently of native encoding.
			sort.Sort(types.Libraries(gotLibs))
			sort.Sort(types.Dependencies(gotDeps))
			assert.Equalf(t, tt.wantLibs, gotLibs, "Parse(%v)", tt.file)
			assert.Equalf(t, tt.wantDeps, gotDeps, "Parse(%v)", tt.file)
		})
	}
}
