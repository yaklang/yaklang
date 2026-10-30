package pcapdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPCAPDBCaptureOverview(t *testing.T) {
	manager := testManager(t)
	db, err := manager.GetOrCreate(writeCapture(t, classicCapture(t, 4)), WithProtocols(true))
	require.NoError(t, err)

	overview, err := db.CaptureOverview()
	require.NoError(t, err)
	require.NotNil(t, overview.FirstTimestampNS)
	require.NotNil(t, overview.LastTimestampNS)
	require.Equal(t, time.Unix(1700000000, 123456789).UnixNano(), *overview.FirstTimestampNS)
	require.Equal(t, time.Unix(1700000003, 123456789).UnixNano(), *overview.LastTimestampNS)
	require.EqualValues(t, 1, overview.ProtocolKindCount)
	require.Equal(t, []ProtocolOverviewCount{{Protocol: "dns", Count: 4}}, overview.Protocols)
	require.Equal(t, []ProtocolStatusOverviewCount{{Status: "decoded", Count: 4}}, overview.Statuses)
	require.Equal(t, []TransportOverviewCount{{Transport: "udp", Count: 4}}, overview.Transports)
	require.Equal(t, []int{1}, overview.LinkTypes)
	require.EqualValues(t, 1, overview.SectionCount)
	require.EqualValues(t, 1, overview.InterfaceCount)
	require.False(t, overview.ProtocolsTruncated)
	require.False(t, overview.StatusesTruncated)
	require.False(t, overview.TransportsTruncated)

	_, err = db.CaptureOverview(QueryTransport("tcp"))
	require.ErrorContains(t, err, "capture overview accepts only")
}

func TestPCAPDBCaptureOverviewWithoutProtocolIndex(t *testing.T) {
	manager := testManager(t)
	db, err := manager.GetOrCreate(writeCapture(t, classicCapture(t, 2)))
	require.NoError(t, err)

	overview, err := db.CaptureOverview()
	require.NoError(t, err)
	require.NotNil(t, overview.FirstTimestampNS)
	require.Empty(t, overview.Protocols)
	require.Empty(t, overview.Statuses)
	require.Empty(t, overview.Transports)
	require.Zero(t, overview.ProtocolKindCount)
	require.Equal(t, []int{1}, overview.LinkTypes)
}
