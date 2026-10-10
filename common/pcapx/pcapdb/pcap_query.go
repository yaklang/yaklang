package pcapdb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/gorm"
)

type queryConfig struct {
	ctx                                          context.Context
	limit                                        int
	after                                        int64
	transport, protocol, sourceIP, destinationIP string
	sourcePort, destinationPort                  *int
	flow                                         *int64
	session, stream                              *uint
	maxBytes                                     int
	start, end                                   *int64
	fields                                       []protocolFieldPredicate
	includeFields                                bool
}
type QueryOption func(*queryConfig) error

func QueryContext(ctx context.Context) QueryOption {
	return func(c *queryConfig) error {
		if ctx == nil {
			return fmt.Errorf("pcapdb: nil query context")
		}
		c.ctx = ctx
		return nil
	}
}
func QueryLimit(limit int) QueryOption {
	return func(c *queryConfig) error {
		if limit < 1 || limit > 1000 {
			return fmt.Errorf("pcapdb: query limit must be 1..1000")
		}
		c.limit = limit
		return nil
	}
}

// QueryWithFields opts into decoding message fields and session snapshots.
// JSON numbers remain json.Number so 64-bit protocol values do not round through
// float64. QueryField accepts these values directly within SQLite's value range.
// Ordinary message lists return summaries without materializing JSON trees.
func QueryWithFields(enabled bool) QueryOption {
	return func(c *queryConfig) error { c.includeFields = enabled; return nil }
}
func QueryAfter(id int64) QueryOption {
	return func(c *queryConfig) error {
		if id < 0 {
			return fmt.Errorf("pcapdb: negative cursor")
		}
		c.after = id
		return nil
	}
}
func QueryTransport(value string) QueryOption {
	return func(c *queryConfig) error { c.transport = strings.ToLower(value); return nil }
}
func QueryProtocol(value string) QueryOption {
	return func(c *queryConfig) error { c.protocol = strings.ToLower(value); return nil }
}
func QuerySourceIP(value string) QueryOption {
	return func(c *queryConfig) error { c.sourceIP = value; return nil }
}
func QueryDestinationIP(value string) QueryOption {
	return func(c *queryConfig) error { c.destinationIP = value; return nil }
}
func QuerySourcePort(value int) QueryOption      { return queryPort(value, true) }
func QueryDestinationPort(value int) QueryOption { return queryPort(value, false) }
func queryPort(value int, source bool) QueryOption {
	return func(c *queryConfig) error {
		if value < 0 || value > 65535 {
			return fmt.Errorf("pcapdb: port must be 0..65535")
		}
		if source {
			c.sourcePort = &value
		} else {
			c.destinationPort = &value
		}
		return nil
	}
}
func QueryFlow(id int64) QueryOption {
	return func(c *queryConfig) error {
		if id < 0 {
			return fmt.Errorf("pcapdb: negative flow ID")
		}
		c.flow = &id
		return nil
	}
}

// QuerySession/QueryStream select indexed associations, not endpoint guesses.
func QuerySession(id uint) QueryOption {
	return func(c *queryConfig) error {
		if id == 0 {
			return fmt.Errorf("pcapdb: zero session ID")
		}
		c.session = &id
		return nil
	}
}
func QueryStream(id uint) QueryOption {
	return func(c *queryConfig) error {
		if id == 0 {
			return fmt.Errorf("pcapdb: zero stream ID")
		}
		c.stream = &id
		return nil
	}
}

// QueryMaxBytes caps ReadStream's response. One complete stored chunk must fit.
func QueryMaxBytes(size int) QueryOption {
	return func(c *queryConfig) error {
		if size < streamChunkBytes || size > 1<<20 {
			return fmt.Errorf("pcapdb: stream page byte budget must be 64 KiB..1 MiB")
		}
		c.maxBytes = size
		return nil
	}
}

func QueryTimeRange(start, end int64) QueryOption {
	return func(c *queryConfig) error {
		if end < start {
			return fmt.Errorf("pcapdb: inverted time range")
		}
		c.start, c.end = &start, &end
		return nil
	}
}

// QueryField matches a typed scalar at a SQLite JSON path. Repeated options
// form an AND query. nil matches an explicit JSON null, not an absent field.
// WithFieldIndex can accelerate a frequently searched path; other paths remain
// searchable but may scan the protocol/time/flow candidate rows.
func QueryField(path string, value any) QueryOption {
	return func(c *queryConfig) error {
		value, err := protocolFieldValue(value)
		if err != nil {
			return err
		}
		return c.addField(protocolFieldPredicate{path: path, value: value})
	}
}

// QueryFieldExists matches any present JSON value, including null and containers.
func QueryFieldExists(path string) QueryOption {
	return func(c *queryConfig) error {
		return c.addField(protocolFieldPredicate{path: path, existence: 1})
	}
}

// QueryFieldMissing matches an absent path, separately from an explicit JSON null.
func QueryFieldMissing(path string) QueryOption {
	return func(c *queryConfig) error {
		return c.addField(protocolFieldPredicate{path: path, existence: -1})
	}
}

func parseQuery(options []QueryOption, messages bool) (*queryConfig, error) {
	c := &queryConfig{ctx: context.Background(), limit: 100, maxBytes: 256 << 10}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("pcapdb: nil query option")
		}
		if err := option(c); err != nil {
			return nil, err
		}
	}
	if messages {
		if c.sourceIP != "" || c.destinationIP != "" || c.sourcePort != nil || c.destinationPort != nil {
			return nil, fmt.Errorf("pcapdb: endpoint IP/port filters apply to packet queries")
		}
	} else if c.protocol != "" || c.flow != nil || len(c.fields) != 0 {
		return nil, fmt.Errorf("pcapdb: protocol/flow/field filters apply to protocol queries")
	}
	return c, nil
}

func (c *queryConfig) apply(db *gorm.DB) *gorm.DB {
	query := db.Where("id > ?", c.after).Order("id").Limit(c.limit)
	if c.transport != "" {
		query = query.Where("transport = ?", c.transport)
	}
	if c.start != nil {
		query = query.Where("timestamp_ns >= ? AND timestamp_ns < ?", *c.start, *c.end)
	}
	if c.protocol != "" {
		query = query.Where("protocol = ?", c.protocol)
	}
	if c.flow != nil {
		query = query.Where("flow_id = ?", *c.flow)
	}
	for _, field := range c.fields {
		query = field.apply(query)
	}
	if c.sourceIP != "" {
		query = query.Where("src_ip = ?", c.sourceIP)
	}
	if c.destinationIP != "" {
		query = query.Where("dst_ip = ?", c.destinationIP)
	}
	if c.sourcePort != nil {
		query = query.Where("src_port = ?", *c.sourcePort)
	}
	if c.destinationPort != nil {
		query = query.Where("dst_port = ?", *c.destinationPort)
	}
	return query
}

func (d *Database) QueryPackets(options ...QueryOption) ([]Packet, error) {
	c, err := parseQuery(options, false)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, false, c.session != nil || c.stream != nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	packets := make([]Packet, 0)
	scoped := indexWithContext(ctx, tx)
	if c.session == nil && c.stream == nil {
		err = c.apply(scoped).Select(packetSummaryColumns).Find(&packets).Error
		return packets, err
	}
	rows, err := packetAssociationQuery(scoped, c).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var packet Packet
		if err = scoped.ScanRows(rows, &packet); err != nil {
			return nil, err
		}
		packets = append(packets, packet)
	}
	err = rows.Err()
	return packets, err
}

// Start at the association cursor and probe each packet's primary key. CROSS
// JOIN fixes this order in SQLite; IN(SELECT all packet IDs) can materialize a
// million-ID stream before applying the page limit. All filters remain in GORM.
func packetAssociationQuery(db *gorm.DB, c *queryConfig) *gorm.DB {
	columns := strings.Split(packetSummaryColumns, ",")
	for i := range columns {
		columns[i] = "p." + columns[i]
	}
	query := db.Table("session_packets AS sp").Joins("CROSS JOIN packets AS p ON p.id=sp.packet_id").Select(strings.Join(columns, ",")).
		Where("sp.deleted_at IS NULL AND p.deleted_at IS NULL AND sp.packet_id > ?", c.after).Order("sp.packet_id").Limit(c.limit)
	if c.session != nil {
		query = query.Where("sp.session_id = ?", *c.session)
	}
	if c.stream != nil {
		query = query.Where("sp.stream_id = ?", *c.stream)
	}
	if c.transport != "" {
		query = query.Where("p.transport = ?", c.transport)
	}
	if c.start != nil {
		query = query.Where("p.timestamp_ns >= ? AND p.timestamp_ns < ?", *c.start, *c.end)
	}
	if c.sourceIP != "" {
		query = query.Where("p.src_ip = ?", c.sourceIP)
	}
	if c.destinationIP != "" {
		query = query.Where("p.dst_ip = ?", c.destinationIP)
	}
	if c.sourcePort != nil {
		query = query.Where("p.src_port = ?", *c.sourcePort)
	}
	if c.destinationPort != nil {
		query = query.Where("p.dst_port = ?", *c.destinationPort)
	}
	return query
}

// The state check and result rows share a read snapshot. A rebuild may commit
// concurrently, but one query cannot mix a Ready checkpoint with partial rows.
func (d *Database) readSnapshot(ctx context.Context, protocols, streams bool) (*gorm.DB, *PCAPFileDBMetadata, error) {
	tx := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if tx.Error != nil {
		return nil, nil, tx.Error
	}
	meta, err := readManifest(ctx, tx, d.ID)
	if err == nil && meta.State != StateReady {
		err = fmt.Errorf("%w: %s", ErrNotReady, meta.State)
	}
	if err == nil && protocols && !meta.ProtocolsIndexed {
		err = fmt.Errorf("pcapdb: protocols have not been indexed; import with withProtocols(true)")
	}
	if err == nil && streams && !meta.StreamsIndexed {
		err = fmt.Errorf("pcapdb: sessions and streams have not been indexed; import with withStreams(true)")
	}
	if err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	return tx, meta, nil
}

// ReadPacket materializes one bounded packet BLOB; packet lists never load it.
func (d *Database) ReadPacket(id uint) ([]byte, error) {
	return d.readData(id, false)
}
func (d *Database) readData(id uint, protocol bool) ([]byte, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(context.Background())
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, protocol, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	model := interface{}(&Packet{})
	lengthColumn := "captured_length"
	if protocol {
		model = &PCAPProtocolMessage{}
		lengthColumn = "data_length"
	}
	var row struct {
		ID     uint
		Length int
		Data   []byte
	}
	err = indexWithContext(ctx, tx).Model(model).Select("id,"+lengthColumn+" AS length,CASE WHEN length(data) <= 16777216 THEN data END AS data").Where("id = ?", id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, ErrNotFound
	}
	if row.Data == nil || row.Length != len(row.Data) {
		return nil, fmt.Errorf("pcapdb: invalid packet/protocol BLOB")
	}
	return row.Data, nil
}

const packetSummaryColumns = "id,created_at,updated_at,deleted_at,capture_interface_id,section,interface_id,link_type,captured_length,original_length,timestamp_ns,src_ip,dst_ip,src_port,dst_port,transport,decode_error"

// Summary projections avoid loading JSON blobs unless explicitly requested.
const protocolSummaryColumns = "id,created_at,updated_at,deleted_at,event_id,flow_id,timestamp_ns,protocol,transport,source,destination,direction,logical_offset,length,status,summary,error,decode_error,profile,rule,entry,completeness,expert_code,transaction_id,response_to,section,interface_id,encapsulation,data_length,session_id,stream_id"

func (d *Database) QueryProtocols(options ...QueryOption) ([]ProtocolMessage, error) {
	c, err := parseQuery(options, true)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, true, c.session != nil || c.stream != nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	projection := protocolSummaryColumns
	if c.includeFields {
		projection += ",json(fields) AS fields,json(session) AS session"
	}
	scoped := indexWithContext(ctx, tx)
	query := c.apply(scoped)
	if c.session != nil {
		query = query.Where("session_id = ?", *c.session)
	}
	if c.stream != nil {
		query = query.Where("stream_id = ?", *c.stream)
	}
	rows, err := query.Model(&PCAPProtocolMessage{}).Select(projection).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]ProtocolMessage, 0)
	var fieldBytes int
	for rows.Next() {
		var message ProtocolMessage
		if err := scoped.ScanRows(rows, &message); err != nil {
			return nil, err
		}
		if c.includeFields {
			fieldBytes += len(message.FieldsJSON) + len(message.SessionJSON)
			if fieldBytes > 16<<20 {
				return nil, fmt.Errorf("pcapdb: field result exceeds 16 MiB; reduce the query limit or omit fields")
			}
			if err := decodeProtocolFields(message.FieldsJSON, &message.Fields); err != nil {
				return nil, err
			}
			if err := decodeProtocolFields(message.SessionJSON, &message.Session); err != nil {
				return nil, err
			}
		}
		message.FieldsJSON, message.SessionJSON, message.SourceBytesJSON = nil, nil, nil
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func decodeProtocolFields(raw []byte, fields *map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(fields)
}

func (d *Database) ProtocolDetails(id uint) (*ProtocolMessage, error) {
	if id < 1 {
		return nil, ErrNotFound
	}
	messages, err := d.QueryProtocols(QueryAfter(int64(id)-1), QueryLimit(1), QueryWithFields(true))
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 || messages[0].ID != id {
		return nil, ErrNotFound
	}
	return &messages[0], nil
}

func (d *Database) ReadProtocol(id uint) ([]byte, error) { return d.readData(id, true) }

// ProtocolPacketIDs is cursor paginated; callers can request the next page
// using QueryAfter(lastPacketID). At most 1000 associations are returned.
func (d *Database) ProtocolPacketIDs(id uint, options ...QueryOption) ([]int64, error) {
	c, err := parseQuery(append([]QueryOption{QueryLimit(1000)}, options...), true)
	if err != nil {
		return nil, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	ctx, cancel := d.manager.operationContext(c.ctx)
	defer cancel()
	tx, _, err := d.readSnapshot(ctx, true, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ids := make([]int64, 0)
	err = indexWithContext(ctx, tx).Model(&PCAPMessagePacket{}).Where("message_id = ? AND packet_id > ?", id, c.after).Order("packet_id").Limit(c.limit).Pluck("packet_id", &ids).Error
	return ids, err
}
