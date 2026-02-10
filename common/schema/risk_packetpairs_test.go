package schema

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// Test that PacketPairs (httpflow_id + url) is persisted to and loaded from the database.
func TestRiskPacketPairsPersisted(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&Risk{}).Error)

	orig := &Risk{
		Title: "packet-pairs-test",
		PacketPairs: PacketPairList{
			{HTTPFlowId: 1, Url: "http://example.com/a"},
			{HTTPFlowId: 2, Url: "http://example.com/b"},
		},
	}

	require.NoError(t, db.Create(orig).Error)

	var got Risk
	require.NoError(t, db.First(&got, orig.ID).Error)

	require.Len(t, got.PacketPairs, 2, "PacketPairs should be persisted in DB")
	require.Equal(t, int64(1), got.PacketPairs[0].HTTPFlowId)
	require.Equal(t, "http://example.com/a", got.PacketPairs[0].Url)
	require.Equal(t, int64(2), got.PacketPairs[1].HTTPFlowId)
	require.Equal(t, "http://example.com/b", got.PacketPairs[1].Url)
}

