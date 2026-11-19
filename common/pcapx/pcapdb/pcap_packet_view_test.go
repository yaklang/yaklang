package pcapdb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestPacketViewFiltersCountOriginalPackets(t *testing.T) {
	m := testManager(t)
	input := writeCapture(t, classicCapture(t, 3))
	db, err := m.GetOrCreate(input, WithProtocols(true), WithStreams(false))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	var manualMessageIDs []uint
	for i := 0; i < 3; i++ {
		source, err := json.Marshal(pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 1}, {Number: 2}}, CompletedIn: &pcaputil.PacketReference{Number: 2}})
		require.NoError(t, err)
		message := PCAPProtocolMessage{EventID: int64(100 + i), Protocol: "http", Transport: "tcp", Summary: "GET /multi-message", Status: "complete", Completeness: "complete", FieldsJSON: []byte(`{"Code":200}`), SessionJSON: []byte(`{}`), SourceBytesJSON: source, Data: []byte("HTTP"), DataLength: 4, CompletedInPacketID: 2}
		require.NoError(t, writer.Create(&message).Error)
		manualMessageIDs = append(manualMessageIDs, message.ID)
		for _, packetID := range []uint{1, 2} {
			require.NoError(t, writer.Create(&PCAPMessagePacket{MessageID: message.ID, PacketID: packetID}).Error)
		}
	}
	terminalOnlySource, err := json.Marshal(pcaputil.ByteSource{Kind: "reassembled", PacketRefs: []pcaputil.PacketReference{{Number: 1}}, CompletedIn: &pcaputil.PacketReference{Number: 2}})
	require.NoError(t, err)
	terminalOnly := PCAPProtocolMessage{EventID: 200, Protocol: "http", Transport: "tcp", Summary: "GET /terminal-only", Status: "complete", Completeness: "complete", FieldsJSON: []byte(`{"Code":200}`), SessionJSON: []byte(`{}`), SourceBytesJSON: terminalOnlySource, Data: []byte("HTTP-terminal"), DataLength: len("HTTP-terminal"), CompletedInPacketID: 2}
	require.NoError(t, writer.Create(&terminalOnly).Error)
	require.NoError(t, writer.Create(&PCAPMessagePacket{MessageID: terminalOnly.ID, PacketID: 1}).Error)
	require.NoError(t, writer.Close())
	handle, err := m.Acquire(context.Background(), db.ID)
	require.NoError(t, err)
	defer handle.Close()
	first, err := handle.QueryPacketView(PacketViewFilter{Protocols: []string{"http"}, Limit: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, first.Total, "only the completing frame belongs to the application protocol")
	require.False(t, first.HasMore)
	require.Len(t, first.Packets, 1)
	require.EqualValues(t, 2, first.Packets[0].ID)
	require.Equal(t, "HTTP", first.Packets[0].Protocol)
	require.Equal(t, "GET /multi-message", first.Packets[0].Info)
	require.Equal(t, []string{"HTTP"}, first.Packets[0].RelatedProtocols)
	require.Nil(t, first.Packets[0].Data, "lists must not load packet BLOBs")
	second, err := handle.QueryPacketView(PacketViewFilter{Protocols: []string{"http"}, AfterID: 1, Limit: 1})
	require.NoError(t, err)
	require.Zero(t, second.Total, "cursor pages do not repeat the filtered count")
	require.Len(t, second.Packets, 1)
	require.EqualValues(t, 2, second.Packets[0].ID)
	require.Equal(t, "HTTP", second.Packets[0].Protocol)
	require.Equal(t, "GET /multi-message", second.Packets[0].Info)
	require.Equal(t, []string{"HTTP"}, second.Packets[0].RelatedProtocols)
	require.False(t, second.HasMore)
	keyword, err := handle.QueryPacketView(PacketViewFilter{Keyword: "/multi-message", Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, keyword.Total)
	require.EqualValues(t, 2, keyword.Packets[0].ID)
	literal, err := handle.QueryPacketView(PacketViewFilter{Keyword: "%' OR 1=1 --", Limit: 10})
	require.NoError(t, err)
	require.Zero(t, literal.Total)
	bySummary, err := handle.QueryPacketView(PacketViewFilter{Keyword: "DstPort=53", Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 3, bySummary.Total)
	messages, err := handle.PacketMessages(2, 1<<20)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(messages), 4)
	var foundTerminalOnly bool
	for _, message := range messages {
		if message.ID == terminalOnly.ID {
			foundTerminalOnly = true
			require.EqualValues(t, 2, message.CompletedInPacketID)
			require.Equal(t, "HTTP-terminal", string(message.Data))
			require.Equal(t, len("HTTP-terminal"), message.DataLength)
		}
	}
	require.True(t, foundTerminalOnly, "terminal frame details must include messages completed by that frame")
	_, err = handle.PacketMessages(2, 1)
	require.ErrorIs(t, err, ErrResultTooLarge)

	completed, related, err := handle.PacketDetailMessages(2, 1<<20)
	require.NoError(t, err)
	completedIDs := make(map[uint]bool, len(completed))
	for _, message := range completed {
		completedIDs[message.ID] = true
		require.Nil(t, message.Data, "detail metadata query must not materialize message BLOBs")
	}
	require.True(t, completedIDs[terminalOnly.ID])
	for _, messageID := range manualMessageIDs {
		require.True(t, completedIDs[messageID], "all messages completed by one frame must be returned")
	}
	for _, message := range related {
		require.NotEqualValues(t, 2, message.CompletedInPacketID)
	}
}

func TestPacketViewHandleCancellation(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 1)), WithProtocols(false))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	handle, err := m.Acquire(ctx, db.ID)
	require.NoError(t, err)
	defer handle.Close()
	cancel()
	_, err = handle.QueryPacketView(PacketViewFilter{Limit: 1})
	require.ErrorIs(t, err, context.Canceled)
}

func TestPacketDetailLoadsOnlyNeededSnapshots(t *testing.T) {
	m := testManager(t)
	db, err := m.GetOrCreate(writeCapture(t, classicCapture(t, 2)), WithProtocols(true), WithStreams(false))
	require.NoError(t, err)
	meta, err := db.Metadata()
	require.NoError(t, err)
	writer, err := openIndex(meta.DatabasePath, true, false)
	require.NoError(t, err)
	defer writer.Close()
	require.False(t, writer.Dialect().HasColumn("protocol_messages", "display_fields"))
	// A frame contributing to a large message must still open without
	// fetching, budgeting or decoding the terminal message's parser snapshots.
	msg := PCAPProtocolMessage{EventID: 500, Protocol: "smb", FieldsJSON: []byte(`{}`),
		SessionJSON: []byte(`{"large":"` + strings.Repeat("x", 32<<10) + `"}`),
		Data:        []byte{1}, DataLength: 1, SourceBytesJSON: []byte(`{}`), CompletedInPacketID: 2}
	require.NoError(t, writer.Create(&msg).Error)
	require.NoError(t, writer.Create(&PCAPMessagePacket{MessageID: msg.ID, PacketID: 1}).Error)
	handle, err := m.Acquire(context.Background(), db.ID)
	require.NoError(t, err)
	defer handle.Close()
	_, links, err := handle.PacketDetailMessages(1, 16<<10)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, msg.ID, links[0].ID)
	require.Nil(t, links[0].FieldsJSON)
	require.Nil(t, links[0].SessionJSON)
	_, _, err = handle.PacketDetailMessages(2, 16<<10)
	require.ErrorIs(t, err, ErrResultTooLarge)
}
