package pcapdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/gorm"
	"strings"
	"unicode/utf8"
)

// ResultPage is the JSON contract for bounded query adapters. Items never
// contain packet/message/stream BLOBs. NextCursor is the last returned record,
// not an offset or count; repeat the same filters and use after(NextCursor).
// Discovery uses message IDs and reports per-page sampled coverage explicitly.
// Errors are also returned as Go errors; adapters can serialize Error directly.
type ResultPage[T any] struct {
	requestCursor int64
	DatasetID     string       `json:"dataset_id"`
	State         State        `json:"state"`
	Items         []T          `json:"items"`
	NextCursor    int64        `json:"next_cursor"`
	CursorKind    string       `json:"cursor_kind"`
	HasMore       bool         `json:"has_more"`
	Truncated     bool         `json:"truncated"`
	Error         *ResultError `json:"error"`
	Scanned       int          `json:"scanned,omitempty"`
	Skipped       int          `json:"skipped,omitempty"`
	Sampled       bool         `json:"sampled,omitempty"`
}

type ResultError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

var ErrResultTooLarge = errors.New("pcapdb: one result exceeds the output budget")

// QueryResultBytes bounds the complete JSON envelope, including escaping and
// base64 expansion. Defaults to 64 KiB; callers can select 4 KiB..1 MiB.
func QueryResultBytes(size int) QueryOption {
	return func(c *queryConfig) error {
		if size < 4096 || size > 1<<20 {
			return fmt.Errorf("pcapdb: result byte budget must be 4 KiB..1 MiB")
		}
		c.resultBytes = size
		return nil
	}
}

// QueryPreviewBytes bounds raw BLOB previews (512 bytes by default). Complete
// binary content remains available via cancellable file export or native reads.
func QueryPreviewBytes(size int) QueryOption {
	return func(c *queryConfig) error {
		if size < 0 || size > 64<<10 {
			return fmt.Errorf("pcapdb: preview must be 0..64 KiB")
		}
		c.previewBytes = size
		return nil
	}
}

func resultError(err error) *ResultError {
	if err == nil {
		return nil
	}
	code, retryable := "query_failed", false
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code, retryable = "deadline_exceeded", true
	case errors.Is(err, context.Canceled):
		code = "canceled"
	case errors.Is(err, ErrClosed):
		code = "closed"
	case errors.Is(err, ErrBusy):
		code, retryable = "busy", true
	case errors.Is(err, ErrNotReady):
		code = "not_ready"
	case errors.Is(err, ErrNotFound):
		code = "not_found"
	case errors.Is(err, ErrAmbiguous):
		code = "ambiguous"
	case errors.Is(err, ErrUnsupportedSchema):
		code = "unsupported_schema"
	case errors.Is(err, ErrResultTooLarge):
		code = "result_too_large"
	}
	return &ResultError{Code: code, Message: shortText(err.Error(), 512), Retryable: retryable}
}

func emptyPage[T any](id string, after int64, kind string) *ResultPage[T] {
	return &ResultPage[T]{requestCursor: after, DatasetID: id, State: "unknown", Items: make([]T, 0), NextCursor: after, CursorKind: kind}
}

func failPage[T any](page *ResultPage[T], err error) (*ResultPage[T], error) {
	page.Error = resultError(err)
	page.NextCursor = page.requestCursor
	// Never expose partial results as a successful page or advance a cursor on
	// failure. The next successful request can retry the same input cursor.
	page.Items = make([]T, 0)
	page.HasMore = false
	return page, err
}

// Reserve enough envelope space while constructing a page, then measure the
// actual final envelope once. This keeps serialization work linear in items.
func pageItems[T any](page *ResultPage[T], rows []T, limit, budget int, id func(T) int64) (*ResultPage[T], error) {
	envelope, err := json.Marshal(page)
	if err != nil {
		return failPage(page, err)
	}
	used := len(envelope) + 32 // cursor digits and boolean changes
	start := page.NextCursor
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		if err != nil {
			return failPage(page, err)
		}
		if len(page.Items) == limit || used+len(encoded)+1 > budget {
			page.HasMore = true
			page.Truncated = len(page.Items) < limit
			if len(page.Items) == 0 {
				page.NextCursor = start
				return failPage(page, ErrResultTooLarge)
			}
			break
		}
		page.Items = append(page.Items, row)
		page.NextCursor = id(row)
		used += len(encoded) + 1
	}
	encoded, err := json.Marshal(page)
	if err == nil && len(encoded) > budget {
		err = ErrResultTooLarge
	}
	if err != nil {
		page.NextCursor = start
		return failPage(page, err)
	}
	return page, nil
}

func shortText(s string, max int) string {
	if len(s) <= max {
		return strings.ToValidUTF8(s, "\uFFFD")
	}
	end := max
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return strings.ToValidUTF8(s[:end], "\uFFFD")
}

// Explicit result models avoid serializing GORM timestamps or persisted JSON
// and keep text previews bounded before they reach an AI tool's output channel.
type PacketSummary struct {
	ID               uint   `json:"id"`
	Section          int64  `json:"section"`
	Interface        int    `json:"interface"`
	TimestampNS      *int64 `json:"timestamp_ns"`
	CapturedLength   int    `json:"captured_length"`
	OriginalLength   int    `json:"original_length"`
	LinkType         int    `json:"link_type"`
	SourceIP         string `json:"source_ip"`
	DestinationIP    string `json:"destination_ip"`
	SourcePort       int    `json:"source_port"`
	DestinationPort  int    `json:"destination_port"`
	Transport        string `json:"transport"`
	DecodeError      string `json:"decode_error,omitempty"`
	PreviewTruncated bool   `json:"preview_truncated,omitempty"`
}
type ProtocolSummary struct {
	ID               uint   `json:"id"`
	Protocol         string `json:"protocol"`
	Transport        string `json:"transport"`
	TimestampNS      *int64 `json:"timestamp_ns"`
	Source           string `json:"source"`
	Destination      string `json:"destination"`
	FlowID           int64  `json:"flow_id"`
	SessionID        uint   `json:"session_id"`
	StreamID         uint   `json:"stream_id"`
	DataLength       int    `json:"data_length"`
	Summary          string `json:"summary"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	Completeness     string `json:"completeness"`
	PreviewTruncated bool   `json:"preview_truncated,omitempty"`
}
type SessionSummary struct {
	ID               uint   `json:"id"`
	Transport        string `json:"transport"`
	SourceIP         string `json:"source_ip"`
	DestinationIP    string `json:"destination_ip"`
	SourcePort       int    `json:"source_port"`
	DestinationPort  int    `json:"destination_port"`
	Section          int64  `json:"section"`
	Interface        int    `json:"interface"`
	FirstTimestampNS *int64 `json:"first_timestamp_ns"`
	LastTimestampNS  *int64 `json:"last_timestamp_ns"`
	PacketCount      int64  `json:"packet_count"`
	ByteCount        int64  `json:"byte_count"`
	Complete         bool   `json:"complete"`
	HasGaps          bool   `json:"has_gaps"`
	Midstream        bool   `json:"midstream"`
	CloseReason      string `json:"close_reason"`
}
type StreamSummary struct {
	ID         uint   `json:"id"`
	SessionID  uint   `json:"session_id"`
	Direction  int    `json:"direction"`
	Kind       string `json:"kind"`
	ByteCount  int64  `json:"byte_count"`
	ChunkCount int64  `json:"chunk_count"`
}

func queryResult[S, T any](d *Database, options []QueryOption, messages bool, query func(...QueryOption) ([]S, error), convert func(S) T, id func(T) int64) (*ResultPage[T], error) {
	page := emptyPage[T](d.ID, 0, "record_id")
	c, err := parseQuery(options, messages)
	if err != nil {
		return failPage(page, err)
	}
	page.NextCursor = c.after
	page.requestCursor = c.after
	if c.includeFields {
		return failPage(page, errors.New("pcapdb: summary pages omit fields; use ProtocolDetailsPage"))
	}
	// Only this internal option may request the one-row lookahead beyond 1000.
	fetch := append(append([]QueryOption(nil), options...), func(q *queryConfig) error { q.limit = c.limit + 1; q.bounded = true; return nil })
	rows, err := query(fetch...)
	if err != nil {
		return failPage(page, err)
	}
	page.State = StateReady // the query validated Ready within its row snapshot
	items := make([]T, 0, len(rows))
	for _, row := range rows {
		items = append(items, convert(row))
	}
	return pageItems(page, items, c.limit, c.resultBytes, id)
}

func (d *Database) QueryPacketsPage(options ...QueryOption) (*ResultPage[PacketSummary], error) {
	return queryResult(d, options, false, d.QueryPackets, func(p Packet) PacketSummary {
		return PacketSummary{ID: p.ID, Section: p.Section, Interface: p.Interface, TimestampNS: p.TimestampNS, CapturedLength: p.CapturedLength, OriginalLength: p.OriginalLength, LinkType: p.LinkType, SourceIP: shortText(p.SourceIP, 64), DestinationIP: shortText(p.DestinationIP, 64), SourcePort: p.SourcePort, DestinationPort: p.DestinationPort, Transport: shortText(p.Transport, 32), DecodeError: shortText(p.DecodeError, 512), PreviewTruncated: len(p.DecodeError) > 512}
	}, func(p PacketSummary) int64 { return int64(p.ID) })
}
func (d *Database) QueryProtocolsPage(options ...QueryOption) (*ResultPage[ProtocolSummary], error) {
	return queryResult(d, options, true, d.QueryProtocols, func(p ProtocolMessage) ProtocolSummary {
		return ProtocolSummary{ID: p.ID, Protocol: shortText(p.Protocol, 128), Transport: shortText(p.Transport, 32), TimestampNS: p.TimestampNS, Source: shortText(p.Source, 256), Destination: shortText(p.Destination, 256), FlowID: p.FlowID, SessionID: p.SessionID, StreamID: p.StreamID, DataLength: p.DataLength, Summary: shortText(p.Summary, 512), Status: shortText(p.Status, 128), Error: shortText(p.Error, 512), Completeness: shortText(p.Completeness, 128), PreviewTruncated: len(p.Summary) > 512 || len(p.Error) > 512 || len(p.Source) > 256 || len(p.Destination) > 256}
	}, func(p ProtocolSummary) int64 { return int64(p.ID) })
}
func (d *Database) QuerySessionsPage(options ...QueryOption) (*ResultPage[SessionSummary], error) {
	return queryResult(d, options, false, d.QuerySessions, sessionSummary, func(p SessionSummary) int64 { return int64(p.ID) })
}
func (d *Database) QueryStreamsPage(options ...QueryOption) (*ResultPage[StreamSummary], error) {
	return queryResult(d, options, false, d.QueryStreams, func(p PCAPStream) StreamSummary {
		return StreamSummary{ID: p.ID, SessionID: p.SessionID, Direction: p.Direction, Kind: shortText(p.Kind, 32), ByteCount: p.ByteCount, ChunkCount: p.ChunkCount}
	}, func(p StreamSummary) int64 { return int64(p.ID) })
}

// DataPreview has explicit binary encoding and size. A truncated preview does
// not claim to be a complete packet/message/chunk; IDs retain access to originals.
type DataPreview struct {
	StreamID           uint   `json:"stream_id,omitempty"`
	ReferencesComplete *bool  `json:"references_complete,omitempty"`
	ID                 uint   `json:"id"`
	Offset             int64  `json:"offset"`
	TotalBytes         int    `json:"total_bytes"`
	Encoding           string `json:"encoding"`
	Data               []byte `json:"data"`
	Truncated          bool   `json:"truncated"`
}

func (d *Database) ReadPacketPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	return d.readPreview(id, false, options)
}
func (d *Database) ReadProtocolPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	return d.readPreview(id, true, options)
}
func (d *Database) readPreview(id uint, protocol bool, options []QueryOption) (*ResultPage[DataPreview], error) {
	page := emptyPage[DataPreview](d.ID, 0, "record_id")
	c, err := parseDetailQuery(options)
	if err != nil {
		return failPage(page, err)
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, protocol, false)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	model := any(&Packet{})
	lengthColumn := "captured_length"
	if protocol {
		model = &PCAPProtocolMessage{}
		lengthColumn = "data_length"
	}
	var row struct {
		ID     uint
		Length int
		Actual int
		Data   []byte
	}
	err = indexWithContext(ctx, tx).Model(model).Select("id,"+lengthColumn+" AS length,length(data) AS actual,substr(data,1,?) AS data", c.previewBytes).Where("id = ?", id).Scan(&row).Error
	if gorm.IsRecordNotFoundError(err) {
		err = ErrNotFound
	}
	if err != nil {
		return failPage(page, err)
	}
	if row.ID == 0 {
		return failPage(page, ErrNotFound)
	}
	if row.Length != row.Actual || row.Length < 0 || row.Length > 16<<20 {
		return failPage(page, fmt.Errorf("pcapdb: invalid packet/protocol BLOB"))
	}
	if row.Data == nil {
		row.Data = []byte{}
	}
	page.State = StateReady
	item := DataPreview{ID: id, TotalBytes: row.Length, Encoding: "base64", Data: row.Data, Truncated: len(row.Data) < row.Length}
	page.Truncated = item.Truncated
	return pageItems(page, []DataPreview{item}, 1, c.resultBytes, func(p DataPreview) int64 { return int64(p.ID) })
}

// ReadStreamPage previews complete stored chunk records without materializing
// the entire stream. NextCursor advances by chunk, including preview-only chunks.
func (d *Database) ReadStreamPage(id uint, options ...QueryOption) (*ResultPage[DataPreview], error) {
	page := emptyPage[DataPreview](d.ID, 0, "chunk_id")
	c, err := parseSummaryQuery(options)
	if err != nil {
		return failPage(page, err)
	}
	page.NextCursor = c.after
	page.requestCursor = c.after
	if c.transport != "" || c.sourceIP != "" || c.destinationIP != "" || c.sourcePort != nil || c.destinationPort != nil || c.start != nil || c.session != nil || c.stream != nil {
		return failPage(page, errors.New("pcapdb: stream previews accept only context/cursor/output options"))
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, false, true)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	var count int
	if err = indexWithContext(ctx, tx).Model(&PCAPStream{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return failPage(page, err)
	}
	if count == 0 {
		return failPage(page, ErrNotFound)
	}
	var rows []struct {
		ID                 uint
		ByteOffset         int64
		Length             int
		Actual             int
		ReferencesComplete bool
		Data               []byte
	}
	fetchLimit := min(c.limit, max(1, c.resultBytes/(c.previewBytes+256)))
	err = indexWithContext(ctx, tx).Model(&PCAPStreamChunk{}).Select("id,byte_offset,length,length(data) AS actual,references_complete,substr(data,1,?) AS data", c.previewBytes).Where("stream_id = ? AND id > ?", id, c.after).Order("id").Limit(fetchLimit + 1).Scan(&rows).Error
	if err != nil {
		return failPage(page, err)
	}
	items := make([]DataPreview, 0, len(rows))
	for _, row := range rows {
		if row.Length != row.Actual || row.Length <= 0 || row.Length > streamChunkBytes {
			return failPage(page, errors.New("pcapdb: invalid stream chunk BLOB"))
		}
		if row.Data == nil {
			row.Data = []byte{}
		}
		complete := row.ReferencesComplete
		preview := DataPreview{StreamID: id, ReferencesComplete: &complete, ID: row.ID, Offset: row.ByteOffset, TotalBytes: row.Length, Encoding: "base64", Data: row.Data, Truncated: len(row.Data) < row.Length}
		items = append(items, preview)
	}
	page.State = StateReady
	result, err := pageItems(page, items, fetchLimit, c.resultBytes, func(p DataPreview) int64 { return int64(p.ID) })
	if fetchLimit < c.limit && result.HasMore {
		result.Truncated = true
	}
	for _, item := range result.Items {
		result.Truncated = result.Truncated || item.Truncated
	}
	return result, err
}

type ProtocolDetail struct {
	ID      uint           `json:"id"`
	Fields  map[string]any `json:"fields"`
	Session map[string]any `json:"session"`
}

// Field trees are returned intact or rejected with result_too_large. The caller
// can discover narrower paths or raise the budget; no invalid JSON is produced.
func (d *Database) ProtocolDetailsPage(id uint, options ...QueryOption) (*ResultPage[ProtocolDetail], error) {
	page := emptyPage[ProtocolDetail](d.ID, 0, "record_id")
	c, err := parseDetailQuery(options)
	if err != nil {
		return failPage(page, err)
	}
	bounded := append(append([]QueryOption(nil), options...), func(q *queryConfig) error { q.fieldBytes = c.resultBytes; return nil })
	detail, err := d.ProtocolDetails(id, bounded...)
	if err != nil {
		return failPage(page, err)
	}
	page.State = StateReady
	return pageItems(page, []ProtocolDetail{{ID: id, Fields: detail.Fields, Session: detail.Session}}, 1, c.resultBytes, func(p ProtocolDetail) int64 { return int64(p.ID) })
}

func jsonMarshalSize(value any) (int, error) {
	encoded, err := json.Marshal(value)
	return len(encoded), err
}

type RecordReference struct {
	ID int64 `json:"id"`
}

func (d *Database) ProtocolPacketIDsPage(id uint, options ...QueryOption) (*ResultPage[RecordReference], error) {
	page, err := queryResult(d, options, true, func(options ...QueryOption) ([]int64, error) { return d.ProtocolPacketIDs(id, options...) }, func(id int64) RecordReference { return RecordReference{ID: id} }, func(ref RecordReference) int64 { return ref.ID })
	page.CursorKind = "packet_id"
	return page, err
}
func (d *Database) StreamChunkPacketIDsPage(id uint, options ...QueryOption) (*ResultPage[RecordReference], error) {
	page, err := queryResult(d, options, false, func(options ...QueryOption) ([]int64, error) { return d.StreamChunkPacketIDs(id, options...) }, func(id int64) RecordReference { return RecordReference{ID: id} }, func(ref RecordReference) int64 { return ref.ID })
	page.CursorKind = "packet_id"
	return page, err
}
func (d *Database) PacketSessionsPage(id uint, options ...QueryOption) (*ResultPage[SessionSummary], error) {
	return queryResult(d, options, false, func(options ...QueryOption) ([]PCAPSession, error) { return d.PacketSessions(id, options...) }, sessionSummary, func(p SessionSummary) int64 { return int64(p.ID) })
}

func sessionSummary(p PCAPSession) SessionSummary {
	return SessionSummary{ID: p.ID, Transport: shortText(p.Transport, 32), SourceIP: shortText(p.SourceIP, 64), DestinationIP: shortText(p.DestinationIP, 64), SourcePort: p.SourcePort, DestinationPort: p.DestinationPort, Section: p.Section, Interface: p.Interface, FirstTimestampNS: p.FirstTimestampNS, LastTimestampNS: p.LastTimestampNS, PacketCount: p.PacketCount, ByteCount: p.ByteCount, Complete: p.Complete, HasGaps: p.HasGaps, Midstream: p.Midstream, CloseReason: shortText(p.CloseReason, 128)}
}
