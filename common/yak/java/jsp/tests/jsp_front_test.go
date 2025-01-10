package tests

import (
	"embed"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"os"
	"strings"
	"testing"
)

//go:embed code
var jspFs embed.FS

func TestJSP_Front(t *testing.T) {
	t.Run("test real  jsp file", func(t *testing.T) {
		efs := filesys.NewEmbedFS(jspFs)
		filesys.Recursive(".",
			filesys.WithEmbedFS(jspFs),
			filesys.WithFileStat(func(s string, info os.FileInfo) error {
				if !strings.HasSuffix(s, ".jsp") {
					return nil
				}
				raw, err := efs.ReadFile(s)
				require.NoError(t, err)
				_, err = jsp.Front(string(raw))
				require.NoError(t, err, "error in file: ", s)
				return nil
			}))
	})
}
