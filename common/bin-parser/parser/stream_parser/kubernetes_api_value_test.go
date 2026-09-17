package stream_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKubernetesAPIIndependentConversionAndSpans(t *testing.T) {
	wrap := func(query string) []byte {
		return []byte("GET /api/v1/namespaces" + query + " HTTP/1.1\r\nHost: arbitrary.example\r\n\r\n")
	}
	for _, query := range []string{"", "?", "?&&", "?=x&unknown=value&", "?watch&limit=2&limit=bad", "?labelSelector=x%3Dy&fieldSelector=a+b&continue=opaque%2Ftoken&resourceVersion=not-a-number"} {
		wire := wrap(query)
		fields, info, err := decodeKubernetesAPIRequest(wire)
		require.NoError(t, err, query)
		require.Equal(t, "Not established", info["Server Identity"])
		var walk func([]kubernetesAPIField, int) int
		walk = func(fields []kubernetesAPIField, pos int) int {
			for _, f := range fields {
				require.Equal(t, pos, f.Start, f.Name)
				require.GreaterOrEqual(t, f.End, f.Start)
				require.LessOrEqual(t, f.End, len(wire))
				if f.Type == "" {
					require.Equal(t, f.End, walk(f.Children, f.Start), f.Name)
				}
				pos = f.End
			}
			return pos
		}
		require.Equal(t, len(wire), walk(fields, 0))
		for cut := 0; cut < len(wire); cut++ {
			_, _, e := decodeKubernetesAPIRequest(wire[:cut])
			require.Error(t, e, "prefix %d", cut)
		}
	}
	// Independent literal oracle for the pinned runtime conversion, not the
	// usual strconv.ParseBool. Semantically invalid watch combinations are still
	// request fields; this decoder never claims server-side acceptance.
	for _, tc := range []struct {
		text string
		want bool
	}{{"false", false}, {"FALSE", false}, {"FaLsE", false}, {"0", false}, {"", true}, {"true", true}, {"1", true}, {"FALSE+", true}, {"arbitrary", true}} {
		for _, key := range []string{"watch", "allowWatchBookmarks", "sendInitialEvents"} {
			_, info, err := decodeKubernetesAPIRequest(wrap("?" + key + "=" + tc.text))
			require.NoError(t, err)
			require.Equal(t, tc.want, info["ListOptions"].(map[string]any)[key])
		}
	}
	for _, tc := range []struct {
		text string
		want int64
	}{{"9223372036854775807", 9223372036854775807}, {"-9223372036854775808", -9223372036854775808}, {"%2B2", 2}, {"-1", -1}, {"000", 0}} {
		_, info, err := decodeKubernetesAPIRequest(wrap("?limit=" + tc.text + "&limit=ignored-invalid"))
		require.NoError(t, err)
		require.Equal(t, tc.want, info["ListOptions"].(map[string]any)["limit"])
	}
	for _, query := range []string{"?limit=9223372036854775808", "?timeoutSeconds=-9223372036854775809", "?limit=", "?limit=+1", "?limit=1.0", "?limit=0x10", "?limit=bad&limit=1", "?watch=%x0", "?%xx=1", "?x=a;b"} {
		_, _, err := decodeKubernetesAPIRequest(wrap(query))
		require.Error(t, err, query)
	}
	for _, count := range []int{256, 257} {
		query := "?" + strings.TrimSuffix(strings.Repeat("unknown=x&", count), "&")
		_, _, err := decodeKubernetesAPIRequest(wrap(query))
		if count == 256 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "occurrence count")
		}
	}
	for _, size := range []int{65536, 65537} {
		prefix := "GET /api/v1/namespaces HTTP/1.1\r\nHost: arbitrary.example\r\nX-Literal: "
		wire := []byte(prefix + strings.Repeat("x", size-len(prefix)-4) + "\r\n\r\n")
		_, _, err := decodeKubernetesAPIRequest(wire)
		if size == 65536 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "size outside")
		}
	}
	for _, count := range []int{256, 257} {
		wire := []byte("GET /api/v1/namespaces HTTP/1.1\r\nHost: example\r\n" + strings.Repeat("X-Unknown: value\r\n", count-1) + "\r\n")
		_, _, err := decodeKubernetesAPIRequest(wire)
		if count == 256 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "header count", fmt.Sprint(count))
		}
	}
}
