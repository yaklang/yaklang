// 本文件定义每个 PCAP 独立 SQLite 子库的数据模型。
// 子库由 pcapdb 自己创建和迁移，使用独立 GORM 句柄，保存接口、包索引、
// 协议内容、会话、分块流及包关联；全部字节保存在子库 BLOB 中。
// Yakit profile 主库的登记模型见 pcap_metadata_schema.go。
package pcapdb

import (
	"context"
	"fmt"

	"github.com/yaklang/gorm"
)

// PCAPDatasetInfo 属于流量子库，保存已提交的导入检查点。
// 实例恢复时，使用该记录修正 profile 主库中的登记摘要。
type PCAPDatasetInfo struct {
	gorm.Model
	Metadata string `gorm:"type:text;not null"`
}

func (PCAPDatasetInfo) TableName() string { return "dataset_info" }

// PCAPCaptureInterface 属于流量子库，记录 PCAPNG section 内的捕获接口。
type PCAPCaptureInterface struct {
	gorm.Model
	Section             int64  `gorm:"unique_index:capture_interface_domain;not null"`
	Interface           int    `gorm:"column:interface_id;unique_index:capture_interface_domain;not null"`
	LinkType            int    `gorm:"not null"`
	Snaplen             uint32 `gorm:"not null"`
	TimestampResolution uint8  `gorm:"not null"`
	TimestampOffset     int64  `gorm:"not null"`
	Name                string `gorm:"type:text;not null"`
}

func (PCAPCaptureInterface) TableName() string { return "capture_interfaces" }

// Packet 属于流量子库，保存完整捕获包及可检索摘要。
// ID 保持捕获顺序；列表不加载 Data，ReadPacket 按 ID 读取 BLOB。
type Packet struct {
	gorm.Model
	CaptureInterfaceID int64  `gorm:"type:integer REFERENCES capture_interfaces(id);not null" json:"-"`
	Section            int64  `gorm:"not null" json:"section"`
	Interface          int    `gorm:"column:interface_id;not null" json:"interface"`
	LinkType           int    `gorm:"not null" json:"link_type"`
	Data               []byte `gorm:"type:blob;not null" json:"-"`
	CapturedLength     int    `gorm:"not null" json:"captured_length"`
	OriginalLength     int    `gorm:"not null" json:"original_length"`
	TimestampNS        *int64 `json:"timestamp_ns"`
	SourceIP           string `gorm:"column:src_ip;type:text;not null" json:"source_ip"`
	DestinationIP      string `gorm:"column:dst_ip;type:text;not null" json:"destination_ip"`
	SourcePort         int    `gorm:"column:src_port;not null" json:"source_port"`
	DestinationPort    int    `gorm:"column:dst_port;not null" json:"destination_port"`
	Transport          string `gorm:"type:text;not null" json:"transport"`
	DecodeError        string `gorm:"type:text;not null" json:"decode_error,omitempty"`
}

func (Packet) TableName() string { return "packets" }

// PCAPProtocolMessage 属于流量子库，保存协议消息、flow/事务关联及 JSONB 字段。
// fields 保存 BIN Parser 完整协议字段树；session 保存会话快照；source_bytes 保存来源信息。
// JSONB 是持久化原本，Fields/Session 仅在详情查询时解码，普通列表只加载摘要。
// 字段索引同样只创建在子库中，方案见 PROTOCOL_FIELDS_SEACH.md。
// Data 保存消息重组字节；SessionID/StreamID 关联网络会话和方向流。
type PCAPProtocolMessage struct {
	gorm.Model
	EventID         int64          `gorm:"unique_index;not null" json:"event_id"`
	FlowID          int64          `gorm:"not null" json:"flow_id"`
	TimestampNS     *int64         `json:"timestamp_ns"`
	Protocol        string         `gorm:"type:text;not null" json:"protocol"`
	Transport       string         `gorm:"type:text;not null" json:"transport"`
	Source          string         `gorm:"type:text;not null" json:"source"`
	Destination     string         `gorm:"type:text;not null" json:"destination"`
	Direction       int            `gorm:"not null" json:"direction"`
	LogicalOffset   int64          `gorm:"not null" json:"logical_offset"`
	Length          int            `gorm:"not null" json:"length"`
	Status          string         `gorm:"type:text;not null" json:"status"`
	Summary         string         `gorm:"type:text;not null" json:"summary"`
	Error           string         `gorm:"type:text;not null" json:"error,omitempty"`
	DecodeError     string         `gorm:"type:text;not null" json:"decode_error,omitempty"`
	Fields          map[string]any `gorm:"-" json:"fields"`
	Session         map[string]any `gorm:"-" json:"session"`
	Section         int64          `gorm:"not null"`
	Interface       int            `gorm:"column:interface_id;not null"`
	Encapsulation   string         `gorm:"type:text;not null"`
	Profile         string         `gorm:"type:text;not null"`
	Rule            string         `gorm:"type:text;not null" json:"rule"`
	Entry           string         `gorm:"type:text;not null" json:"entry"`
	Completeness    string         `gorm:"type:text;not null"`
	ExpertCode      string         `gorm:"type:text;not null"`
	TransactionID   int64          `gorm:"not null"`
	ResponseTo      int64          `gorm:"not null"`
	DataLength      int            `gorm:"not null" json:"data_length"`
	Data            []byte         `gorm:"type:blob;not null" json:"-"`
	SessionID       uint           `gorm:"type:integer REFERENCES sessions(id);default:null" json:"session_id,omitempty"`
	StreamID        uint           `gorm:"type:integer REFERENCES streams(id);default:null" json:"stream_id,omitempty"`
	FieldsJSON      []byte         `gorm:"column:fields;type:blob;not null" json:"-"`
	SessionJSON     []byte         `gorm:"column:session;type:blob;not null" json:"-"`
	SourceBytesJSON []byte         `gorm:"column:source_bytes;type:blob;not null" json:"-"`
}

func (PCAPProtocolMessage) TableName() string { return "protocol_messages" }

// ProtocolMessage is the query result type exposed to Go and Yak callers.
type ProtocolMessage = PCAPProtocolMessage

// PCAPMessagePacket 属于流量子库，将重组协议消息关联到物理包记录。
type PCAPMessagePacket struct {
	gorm.Model
	MessageID uint `gorm:"unique_index:message_packet_identity;type:integer REFERENCES protocol_messages(id);not null"`
	PacketID  uint `gorm:"unique_index:message_packet_identity;type:integer REFERENCES packets(id);not null"`
}

func (PCAPMessagePacket) TableName() string { return "message_packets" }

// PCAPCaptureRecord 保存原捕获文件的封装信息。按 ID 顺序导出 Prefix、
// PacketID 对应的完整包 BLOB、Suffix；非包记录只保存 Prefix。
// 包内容只存一份，PCAPNG 的选项、未知块和原始时间精度均可原样恢复。
type PCAPCaptureRecord struct {
	gorm.Model
	PacketID *uint  `gorm:"unique_index;type:integer REFERENCES packets(id)"`
	Prefix   []byte `gorm:"type:blob;not null"`
	Suffix   []byte `gorm:"type:blob;not null"`
}

func (PCAPCaptureRecord) TableName() string { return "capture_records" }

// PCAPSession 是捕获域内的一次双向通信。TCP 的连接重用创建新会话；
// UDP 按端点及空闲窗口分组，不把数据报宣称为可靠字节流。
type PCAPSession struct {
	gorm.Model
	Transport        string `gorm:"type:text;not null"`
	Section          int64  `gorm:"not null"`
	Interface        int    `gorm:"column:interface_id;not null"`
	Encapsulation    string `gorm:"type:text;not null"`
	SourceIP         string `gorm:"column:src_ip;type:text;not null"`
	DestinationIP    string `gorm:"column:dst_ip;type:text;not null"`
	SourcePort       int    `gorm:"column:src_port;not null"`
	DestinationPort  int    `gorm:"column:dst_port;not null"`
	FirstTimestampNS *int64
	LastTimestampNS  *int64
	PacketCount      int64  `gorm:"not null"`
	ByteCount        int64  `gorm:"not null"`
	Midstream        bool   `gorm:"not null"`
	HasGaps          bool   `gorm:"not null"`
	Complete         bool   `gorm:"not null"`
	CloseReason      string `gorm:"type:text;not null"`
}

func (PCAPSession) TableName() string { return "sessions" }

// PCAPStream 是一个会话的一个方向。ByteCount 是已交付的逻辑字节数，
// TCP 序列号不是文件偏移。列表仅返回摘要，内容通过分块接口读取。
type PCAPStream struct {
	gorm.Model
	SessionID  uint   `gorm:"type:integer REFERENCES sessions(id);not null;unique_index:stream_direction"`
	Direction  int    `gorm:"not null;unique_index:stream_direction"`
	Kind       string `gorm:"type:text;not null"` // tcp / datagram
	ByteCount  int64  `gorm:"not null"`
	ChunkCount int64  `gorm:"not null"`
}

func (PCAPStream) TableName() string { return "streams" }

// PCAPStreamChunk 最多保存 64 KiB 字节，逻辑 ByteOffset 仅用于分页和定位。
// UDP 每个数据报独立成块；TCP 只保存重组器确认的顺序字节，不拼接缺口。
type PCAPStreamChunk struct {
	gorm.Model
	StreamID           uint   `gorm:"type:integer REFERENCES streams(id);not null;unique_index:stream_byte_offset"`
	ByteOffset         int64  `gorm:"not null;unique_index:stream_byte_offset"`
	Length             int    `gorm:"not null"`
	ReferencesComplete bool   `gorm:"not null"`
	Sequence           uint32 `gorm:"not null"`
	TimestampNS        *int64
	Data               []byte `gorm:"type:blob;not null" json:"-"`
}

func (PCAPStreamChunk) TableName() string { return "stream_chunks" }

// PCAPSessionPacket 保留包含 SYN/ACK/FIN/RST、重传在内的原包关联。
type PCAPSessionPacket struct {
	gorm.Model
	SessionID uint `gorm:"type:integer REFERENCES sessions(id);not null;unique_index:session_packet_identity"`
	StreamID  uint `gorm:"type:integer REFERENCES streams(id);not null"`
	PacketID  uint `gorm:"type:integer REFERENCES packets(id);not null;unique_index:session_packet_identity"`
}

func (PCAPSessionPacket) TableName() string { return "session_packets" }

// PCAPStreamChunkPacket 将已重组块关联到提供字节的原包，支持反向追踪。
type PCAPStreamChunkPacket struct {
	gorm.Model
	ChunkID  uint `gorm:"type:integer REFERENCES stream_chunks(id);not null;unique_index:chunk_packet_identity"`
	PacketID uint `gorm:"type:integer REFERENCES packets(id);not null;unique_index:chunk_packet_identity"`
}

func (PCAPStreamChunkPacket) TableName() string { return "stream_chunk_packets" }

// migratePCAPDatabase 仅迁移当前流量子库的数据表和查询索引。
// 所有模型和复合索引都由当前子库的 GORM 句柄管理。
func migratePCAPDatabase(db *gorm.DB) error {
	if err := db.AutoMigrate(&PCAPDatasetInfo{}, &PCAPCaptureInterface{}, &Packet{}, &PCAPCaptureRecord{}, &PCAPSession{}, &PCAPStream{}, &PCAPStreamChunk{}, &PCAPSessionPacket{}, &PCAPStreamChunkPacket{}, &PCAPProtocolMessage{}, &PCAPMessagePacket{}).Error; err != nil {
		return err
	}
	// Keep cursor order as the second column of each searchable index.
	indexes := []struct {
		model   interface{}
		name    string
		columns []string
	}{
		{&Packet{}, "packets_time", []string{"timestamp_ns", "id"}},
		{&Packet{}, "packets_transport", []string{"transport", "id"}},
		{&Packet{}, "packets_src_ip", []string{"src_ip", "id"}},
		{&Packet{}, "packets_dst_ip", []string{"dst_ip", "id"}},
		{&Packet{}, "packets_src_port", []string{"src_port", "id"}},
		{&Packet{}, "packets_dst_port", []string{"dst_port", "id"}},
		{&PCAPProtocolMessage{}, "messages_protocol", []string{"protocol", "id"}},
		{&PCAPProtocolMessage{}, "messages_flow", []string{"flow_id", "id"}},
		{&PCAPProtocolMessage{}, "messages_time", []string{"timestamp_ns", "id"}},
		{&PCAPProtocolMessage{}, "messages_transaction", []string{"transaction_id", "id"}},
		{&PCAPMessagePacket{}, "message_packets_packet", []string{"packet_id", "message_id"}},
		{&PCAPProtocolMessage{}, "messages_session", []string{"session_id", "id"}},
		{&PCAPProtocolMessage{}, "messages_stream", []string{"stream_id", "id"}},
		{&PCAPSession{}, "sessions_transport", []string{"transport", "id"}},
		{&PCAPSession{}, "sessions_src_ip", []string{"src_ip", "id"}},
		{&PCAPSession{}, "sessions_dst_ip", []string{"dst_ip", "id"}},
		{&PCAPSession{}, "sessions_src_port", []string{"src_port", "id"}},
		{&PCAPSession{}, "sessions_dst_port", []string{"dst_port", "id"}},
		{&PCAPSession{}, "sessions_time", []string{"first_timestamp_ns", "id"}},
		{&PCAPStreamChunk{}, "chunks_stream_cursor", []string{"stream_id", "id"}},
		{&PCAPStreamChunk{}, "chunks_stream_offset", []string{"stream_id", "byte_offset", "id"}},
		{&PCAPSessionPacket{}, "session_packets_cursor", []string{"session_id", "packet_id"}},
		{&PCAPSessionPacket{}, "stream_packets_cursor", []string{"stream_id", "packet_id"}},
		{&PCAPSessionPacket{}, "session_packets_reverse", []string{"packet_id", "session_id", "stream_id"}},
		{&PCAPStreamChunkPacket{}, "chunk_packets_reverse", []string{"packet_id", "chunk_id"}},
	}
	for _, index := range indexes {
		if err := db.Model(index.model).AddIndex(index.name, index.columns...).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureProtocolFieldIndexes 仅为当前流量子库创建按需的标量字段索引。
// SQLite 自己的 sqlite_schema 保存索引定义，profile 主库不登记协议字段。
// 同一事务提交所有新增索引；取消/失败不会留下部分索引或修改 Ready 数据。
func ensureProtocolFieldIndexes(ctx context.Context, db *gorm.DB, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	tx := db.BeginTx(ctx, nil)
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()
	scoped := indexWithContext(ctx, tx)
	var count int
	if err := scoped.Table("sqlite_schema").Where("type = ? AND tbl_name = ? AND substr(name,1,?) = ?", "index", "protocol_messages", len(protocolFieldIndexPrefix), protocolFieldIndexPrefix).Count(&count).Error; err != nil {
		return err
	}
	changed := false
	for _, path := range paths {
		if err := validateProtocolFieldPath(path); err != nil {
			return err
		}
		name := protocolFieldIndexName(path)
		var existing int
		if err := scoped.Table("sqlite_schema").Where("type = ? AND name = ?", "index", name).Count(&existing).Error; err != nil {
			return err
		}
		if existing != 0 {
			continue
		}
		if count >= maxProtocolFieldIndexes {
			return fmt.Errorf("pcapdb: at most %d protocol field indexes per database are allowed", maxProtocolFieldIndexes)
		}
		// Containers and absent values are excluded from this partial index:
		// each scalar occurrence contributes one value + JSON type + message ID.
		if err := scoped.Model(&PCAPProtocolMessage{}).
			Where("deleted_at IS NULL AND "+protocolFieldScalarCondition(path)).
			AddIndex(name, protocolFieldExpression("json_extract", path), protocolFieldExpression("json_type", path), "id").Error; err != nil {
			return err
		}
		count++
		changed = true
	}
	if changed {
		// Refresh bounded planner statistics after schema changes. SQLite can
		// otherwise prefer the Model.DeletedAt index over a selective field.
		if err := scoped.Exec("PRAGMA optimize=0x10002").Error; err != nil {
			return err
		}
	}
	return tx.Commit().Error
}
