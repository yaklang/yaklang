package schema

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// Test that PacketPairs is persisted to and loaded from the database.
func TestRiskPacketPairsPersisted(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&Risk{}).Error)

	orig := &Risk{
		Title: "packet-pairs-test",
		PacketPairs: PacketPairList{
			{Request: []byte("REQ1"), Response: []byte("RSP1")},
			{Request: []byte("REQ2"), Response: []byte("RSP2")},
		},
	}

	require.NoError(t, db.Create(orig).Error)

	var got Risk
	require.NoError(t, db.First(&got, orig.ID).Error)

	// 当前实现下，这里会失败（PacketPairs 为空），用于暴露问题
	require.Len(t, got.PacketPairs, 2, "PacketPairs should be persisted in DB")
	require.Equal(t, []byte("REQ1"), got.PacketPairs[0].Request)
	require.Equal(t, []byte("RSP1"), got.PacketPairs[0].Response)
}

