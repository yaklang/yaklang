// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package gradle

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name      string
		inputFile string
		want      []types.Library
	}{
		{
			name:      "happy path",
			inputFile: "testdata/happy.lockfile",
			want: []types.Library{
				{
					Name:    "cglib:cglib-nodep",
					Version: "2.1.2",
				},
				{
					Name:    "org.springframework:spring-asm",
					Version: "3.1.3.RELEASE",
				},
				{
					Name:    "org.springframework:spring-beans",
					Version: "5.0.5.RELEASE",
				},
			},
		},
		{
			name:      "empty",
			inputFile: "testdata/empty.lockfile",
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			f, err := os.Open(tt.inputFile)
			assert.NoError(t, err)

			libs, _, err := parser.Parse(nil, f)
			assert.NoError(t, err)
			for i := range libs {
				libs[i] = types.Library{Name: libs[i].Name, Version: libs[i].Version}
			}
			sortLibs(libs)
			assert.Equal(t, tt.want, libs)
		})
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
