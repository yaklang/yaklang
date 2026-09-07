package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSimpleValueFileFilter_ScopeRegion(t *testing.T) {
	files := map[string]string{
		"a.txt": "env:\n  TOKEN: ${{ secrets.A }}\njobs:\n  JOB: ${{ secrets.B }}\n",
	}
	// hit = the env: block region [0, 30)
	sv := NewSimpleValue("env:\n  TOKEN: ${{ secrets.A }}\n", "a.txt", 0, 30)
	sv.SetFiles(files)

	// scope=region: only secrets inside the env block region
	res, err := sv.FileFilter("", "regexp", map[string]string{"scope": "region"}, []string{`\$\{\{\s*secrets\.`})
	require.NoError(t, err)
	require.Equal(t, 1, ValuesLen(res))
	first, _ := res.First()
	require.Equal(t, "a.txt", first.(*SimpleValue).Path())
	require.Contains(t, first.String(), "secrets.")
	require.Less(t, first.(*SimpleValue).Start(), 30) // inside env block region

	// default scope=file: both secrets in the file
	res, err = sv.FileFilter("", "regexp", nil, []string{`\$\{\{\s*secrets\.`})
	require.NoError(t, err)
	require.Equal(t, 2, ValuesLen(res))
}

func TestSimpleValueFileFilter_ChainedNot(t *testing.T) {
	files := map[string]string{
		"a.js": "var a = \"ws://evil.example/\";\nvar b = \"ws://localhost:27017/x\";\n",
	}
	// hit = ws:// at [9,14) — evil, not overlapping localhost line
	evil := NewSimpleValue("ws://", "a.js", 9, 14)
	evil.SetFiles(files)
	res, err := evil.FileFilter("", "regexp", map[string]string{"__sf_pattern_not_list": "1"}, []string{`\bws:\/\/localhost.*`})
	require.NoError(t, err)
	require.Equal(t, 1, ValuesLen(res))

	// hit = ws:// at [39,44) — localhost, overlapping the negative → dropped
	loc := NewSimpleValue("ws://", "a.js", 39, 44)
	loc.SetFiles(files)
	res, err = loc.FileFilter("", "regexp", map[string]string{"__sf_pattern_not_list": "1"}, []string{`\bws:\/\/localhost.*`})
	require.NoError(t, err)
	require.True(t, res.IsEmpty())
}
