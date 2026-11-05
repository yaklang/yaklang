package sharkcli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPResponseLatencyMetadataUsesMicroseconds(t *testing.T) {
	for _, v := range []any{time.Duration(270), int(270), int64(270), uint64(270), float64(270), json.Number("270")} {
		require.Equal(t, "270 us", formatFieldValue("Response Latency", v), "HTTP retains a microsecond count, including its legacy time.Duration Go type")
	}
	require.Equal(t, "-17 us", formatFieldValue("Response Latency", time.Duration(-17)))
	require.Equal(t, "-17 us", formatFieldValue("Response Latency", float64(-17)))
	require.Equal(t, "270ns", formatFieldValue("Other Duration", time.Duration(270)))
	for _, v := range []any{270.5, "270", json.Number("not-a-number")} {
		_, ok := httpLatencyMetadata("Response Latency", v)
		require.False(t, ok, "do not guess units from untyped text or a fractional count")
	}
}
