package java_decompiler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestJavaDecompilerAction_WindowsStyleNestedPaths(t *testing.T) {
	tempJarPath, _ := createTestJarWithNested(t)

	action := NewJavaDecompilerAction()
	t.Cleanup(action.ClearCache)

	t.Run("list nested jar directory", func(t *testing.T) {
		url, err := CreateUrlFromString("javadec:///jar")
		require.NoError(t, err)
		url.Query = append(url.Query,
			&ypb.KVPair{Key: "jar", Value: tempJarPath},
			&ypb.KVPair{Key: "dir", Value: `\lib\nested.jar\com\example`},
		)

		resp, err := action.Get(&ypb.RequestYakURLParams{
			Url:    url,
			Method: "GET",
		})
		require.NoError(t, err)

		found := false
		for _, res := range resp.Resources {
			if strings.Contains(res.Path, "NestedClass") {
				found = true
				break
			}
		}
		require.True(t, found, "should find NestedClass.class in nested jar")
	})

	t.Run("read nested jar class", func(t *testing.T) {
		url, err := CreateUrlFromString("javadec:///class")
		require.NoError(t, err)
		url.Query = append(url.Query,
			&ypb.KVPair{Key: "jar", Value: tempJarPath},
			&ypb.KVPair{Key: "class", Value: `\lib\nested.jar\com\example\NestedClass.class`},
		)

		resp, err := action.Get(&ypb.RequestYakURLParams{
			Url:    url,
			Method: "GET",
		})
		require.NoError(t, err)
		require.Len(t, resp.Resources, 1)
		require.Equal(t, "class", resp.Resources[0].ResourceType)
		require.True(t, strings.HasSuffix(resp.Resources[0].Path, "NestedClass.class"))
	})
}
