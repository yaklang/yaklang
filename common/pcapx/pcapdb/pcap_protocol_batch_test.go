package pcapdb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProtocolCompletionZeroKeepsStableBatchColumns(t *testing.T) {
	db, err := openIndex(filepath.Join(t.TempDir(), "index.sqlite"), true, true)
	require.NoError(t, err)
	defer db.Close()

	field, ok := db.NewScope(&PCAPProtocolMessage{}).FieldByName("CompletedInPacketID")
	require.True(t, ok)
	require.False(t, field.HasDefaultValue,
		"zero completed_in_packet_id must be inserted explicitly so protocol rows share one batch column shape")
}
