// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package pipenv

import (
	"path"
	"sort"
	"strings"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParse(t *testing.T) {
	vectors := []struct {
		file string // Test input file
		want []types.Library
	}{
		{
			file: "testdata/Pipfile_normal.lock",
			want: pipenvNormal,
		},
		{
			file: "testdata/Pipfile_django.lock",
			want: pipenvDjango,
		},
		{
			file: "testdata/Pipfile_many.lock",
			want: pipenvMany,
		},
	}

	for _, v := range vectors {
		t.Run(path.Base(v.file), func(t *testing.T) {
			f, err := fixtures.OpenReader(v.file)
			require.NoError(t, err)

			got, _, err := NewParser().Parse(nil, f)
			require.NoError(t, err)

			for i := range got {
				got[i].Condition = ""
				got[i].Source = ""
			}
			sort.Slice(got, func(i, j int) bool {
				ret := strings.Compare(got[i].Name, got[j].Name)
				if ret == 0 {
					return got[i].Version < got[j].Version
				}
				return ret < 0
			})

			sort.Slice(v.want, func(i, j int) bool {
				ret := strings.Compare(v.want[i].Name, v.want[j].Name)
				if ret == 0 {
					return v.want[i].Version < v.want[j].Version
				}
				return ret < 0
			})

			for i := range got {
				got[i].Verification = ""
				got[i].DeclaredIntegrity = ""
				got[i].Diagnostics = nil
			}
			assert.Equal(t, v.want, got)
		})
	}
}
