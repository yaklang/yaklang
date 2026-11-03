package pcapdb

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

func TestProtocolDetailsBatchPageOrderAssociationsAndErrors(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 3)), WithProtocols(true))
	require.NoError(t, err)

	page, err := db.ProtocolDetailsBatchPage([]uint{3, 1, 999, 2}, QueryResultBytes(65536))
	require.NoError(t, err)
	require.Nil(t, page.Error)
	require.Equal(t, StateReady, page.State)
	require.Equal(t, "record_id_batch", page.CursorKind)
	require.EqualValues(t, 2, page.NextCursor)
	require.False(t, page.HasMore)
	require.False(t, page.Truncated)
	require.Len(t, page.Items, 4)
	require.Equal(t, []uint{3, 1, 999, 2}, []uint{page.Items[0].ID, page.Items[1].ID, page.Items[2].ID, page.Items[3].ID})
	require.Equal(t, "dns", page.Items[0].Protocol)
	require.Equal(t, "udp", page.Items[0].Transport)
	require.NotEmpty(t, page.Items[0].Fields)
	require.NotZero(t, page.Items[0].SessionID)
	require.NotNil(t, page.Items[2].Error)
	require.Equal(t, "not_found", page.Items[2].Error.Code)
	require.Empty(t, page.Items[2].Fields)

	single, err := db.ProtocolDetailsBatchPage([]uint{1}, QueryResultBytes(65536))
	require.NoError(t, err)
	singleDetail, err := db.ProtocolDetails(1)
	require.NoError(t, err)
	require.Equal(t, singleDetail.Fields, single.Items[0].Fields)
	require.Equal(t, singleDetail.Session, single.Items[0].Session)
}

func TestProtocolDetailsBatchPageBudgetKeepsAcceptedItems(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)), WithProtocols(true))
	require.NoError(t, err)
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", 1).
		UpdateColumn("fields", gorm.Expr("jsonb(?)", `{"large":"`+strings.Repeat("x", 9000)+`"}`)).Error)
	require.NoError(t, writer.Close())

	page, err := db.ProtocolDetailsBatchPage([]uint{1, 2}, QueryResultBytes(4096))
	require.NoError(t, err)
	require.Nil(t, page.Error)
	require.True(t, page.Truncated)
	require.Len(t, page.Items, 2)
	require.Equal(t, "result_too_large", page.Items[0].Error.Code)
	require.Nil(t, page.Items[0].Fields)
	require.NotEmpty(t, page.Items[1].Fields)
	encoded, err := json.Marshal(page)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), 4096)
}

func TestProtocolDetailsBatchTokenBudgetStaysBelowToolResultLimit(t *testing.T) {
	require.LessOrEqual(t,
		maxProtocolDetailBatchTokens+protocolDetailBatchReserve,
		aicommon.ToolResultTokenLimit,
		"detail batch budget must leave room for the canonical AI tool result envelope",
	)

	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 20)), WithProtocols(true))
	require.NoError(t, err)
	writer, err := openIndex(db.path, true, false)
	require.NoError(t, err)
	for id := 1; id <= 20; id++ {
		require.NoError(t, writer.Model(&PCAPProtocolMessage{}).Where("id = ?", id).
			UpdateColumn("fields", gorm.Expr("jsonb(?)", fmt.Sprintf(`{"large":%q}`, strings.Repeat("x", 6000)))).Error)
	}
	require.NoError(t, writer.Close())

	ids := makeIDs(20)
	page, err := db.ProtocolDetailsBatchPage(ids, QueryResultBytes(1<<20))
	require.NoError(t, err)
	require.Nil(t, page.Error)
	require.True(t, page.Truncated)
	require.Len(t, page.Items, len(ids))
	for i, item := range page.Items {
		require.Equal(t, ids[i], item.ID)
	}
	var degraded bool
	for _, item := range page.Items {
		if item.Error != nil && item.Error.Code == "result_too_large" {
			degraded = true
			require.Nil(t, item.Fields)
			require.Nil(t, item.Session)
		}
	}
	require.True(t, degraded)

	compact, err := json.Marshal(page)
	require.NoError(t, err)
	require.False(t, ytoken.TokenCountExceeds(string(compact), maxProtocolDetailBatchTokens))
	pretty, err := json.MarshalIndent(page, "", "  ")
	require.NoError(t, err)
	require.False(t, ytoken.TokenCountExceeds(string(pretty), aicommon.ToolResultTokenLimit-1024),
		"pretty tool output plus envelope reserve must remain below the canonical 16K-token limit")
}

func TestProtocolDetailsBatchPageRejectsUnboundedRequests(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)), WithProtocols(true))
	require.NoError(t, err)

	for name, ids := range map[string][]uint{
		"empty":     nil,
		"zero":      {1, 0},
		"duplicate": {1, 1},
		"too-many":  makeIDs(maxProtocolDetailBatchIDs + 1),
	} {
		t.Run(name, func(t *testing.T) {
			page, err := db.ProtocolDetailsBatchPage(ids, QueryResultBytes(4096))
			require.Error(t, err)
			require.NotNil(t, page.Error)
			require.Empty(t, page.Items)
		})
	}
}

func makeIDs(count int) []uint {
	ids := make([]uint, count)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	return ids
}
