package sfweb_test

import (
	"path"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestTemplate(t *testing.T) {
	entries, _ := sfweb.TemplateFS.ReadDir("templates")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, lang := path.Split(entry.Name())
		templateEntries, err := sfweb.TemplateFS.ReadDir(path.Join("templates", lang))
		require.NoError(t, err)
		for _, entrie := range templateEntries {
			if entrie.IsDir() {
				continue
			}
			filename := entrie.Name()
			fullName := path.Join("templates", lang, filename)
			content, err := sfweb.TemplateFS.ReadFile(fullName)
			require.NoError(t, err)
			if strings.Contains(filename, "example") {
				continue
			}

			// test cache
			t.Run(fullName, func(t *testing.T) {
				scanContent(t, lang, string(content))
			})
		}
	}

}

func TestTemplateDebug(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	// name := "templates/golang/cwe-79-xss-unsafe"
	name := "cwe-79-xss-unsafe"
	lang := "golang"
	fullName := path.Join("templates", lang, name)
	content, err := sfweb.TemplateFS.ReadFile(fullName)
	require.NoError(t, err)
	// scanContent(t, lang, string(content))

	progName := uuid.NewString()
	_, err = ssaapi.Parse(
		string(content),
		ssaapi.WithProgramName(progName),
	)
	require.NoError(t, err)

	prog, err := ssaapi.FromDatabase(progName)
	require.NoError(t, err)
	prog.SyntaxFlowWithError(`
	`)
}
