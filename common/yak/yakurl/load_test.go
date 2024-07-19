package yakurl

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCreateUrlFromString(t *testing.T) {
	t.Run("Windows-path", func(t *testing.T) {
		testcases := []struct {
			u          string
			wantSchema string
			wantPath   string
			wantQuery  map[string]string
		}{
			{
				u:          "file://C:/a/b?a=b&c=d",
				wantSchema: "file",
				wantPath:   "C:/a/b",
				wantQuery: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
			{
				u:          "file://C:\\a\\b?a=b&c=d",
				wantSchema: "file",
				wantPath:   "C:\\a\\b",
				wantQuery: map[string]string{
					"a": "b",
					"c": "d",
				},
			},
		}

		for _, testcase := range testcases {
			parsed, err := CreateUrlFromString(testcase.u)
			require.NoError(t, err)
			require.NotNil(t, parsed)
			require.Equal(t, testcase.wantSchema, parsed.GetSchema())
			require.Equal(t, testcase.wantPath, parsed.GetPath())
			q := lo.SliceToMap(parsed.GetQuery(), func(item *ypb.KVPair) (string, string) {
				return item.Key, item.Value
			})
			require.Equal(t, testcase.wantQuery, q)
		}
	})
}
