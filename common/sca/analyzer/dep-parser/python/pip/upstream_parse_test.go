// Derived from aquasecurity/go-dep-parser, MIT. See LICENSE.
package pip

import (
	"path"
	"testing"

	assert "github.com/yaklang/yaklang/common/sca/internal/testcheck"
	require "github.com/yaklang/yaklang/common/sca/internal/testcheck"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

func TestParse(t *testing.T) {
	vectors := []struct {
		file string
		want []types.Library
	}{
		{
			file: "testdata/requirements_flask.txt",
			want: requirementsFlask,
		},
		{
			file: "testdata/requirements_comments.txt",
			want: requirementsComments,
		},
		{
			file: "testdata/requirements_spaces.txt",
			want: requirementsSpaces,
		},
		{
			file: "testdata/requirements_no_version.txt",
			want: requirementsNoVersion,
		},
		{
			file: "testdata/requirements_operator.txt",
			want: requirementsOperator,
		},
		{
			file: "testdata/requirements_hash.txt",
			want: requirementsHash,
		},
		{
			file: "testdata/requirements_hyphens.txt",
			want: requirementsHyphens,
		},
		{
			file: "testdata/requirement_exstras.txt",
			want: requirementsExtras,
		},
	}

	for _, v := range vectors {
		t.Run(path.Base(v.file), func(t *testing.T) {
			f, err := fixtures.OpenReader(v.file)
			require.NoError(t, err)

			got, _, err := NewParser().Parse(nil, f)
			require.NoError(t, err)
			// Preserve the pinned exact-version assertions. New declarations,
			// conditions and provenance have independent assertions below.
			counts := map[string]int{"testdata/requirements_flask.txt": 6, "testdata/requirements_comments.txt": 4, "testdata/requirements_spaces.txt": 4, "testdata/requirements_no_version.txt": 2, "testdata/requirements_operator.txt": 7, "testdata/requirements_hash.txt": 2, "testdata/requirements_hyphens.txt": 2, "testdata/requirement_exstras.txt": 2}
			if len(got) != counts[v.file] {
				t.Fatalf("declaration inventory %d want %d", len(got), counts[v.file])
			}
			var pinned []types.Library
			for _, v := range got {
				if v.Version != "" {
					pinned = append(pinned, types.Library{Name: v.Name, Version: v.Version})
				}
				if v.Evidence != "declared" || v.DeclaredName != v.Name {
					t.Fatal("lost declaration evidence")
				}
			}
			got = pinned

			assert.Equal(t, v.want, got)
		})
	}
}
