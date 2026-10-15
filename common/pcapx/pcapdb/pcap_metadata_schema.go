// 本文件定义 Yakit profile 主库的 PCAP 库登记表。
// 该模型通过 KEY_SCHEMA_PROFILE_DATABASE 注册，由 profile 主库迁移管理。
// 每个独立流量子库的数据模型见 pcap_database_schema.go。
package pcapdb

import (
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// PCAPFileDBMetadata 属于 Yakit profile 主库，TableName 为 pcapfile_db_metadata。
// 主库登记独立子库的位置及原捕获文件的来源、内容指纹、版本、状态及进度摘要。
// FullSHA256 是内容去重依据；Fingerprint 是带版本的候选查找指纹。
// 路径均为绝对路径，SourceAliases 是相同内容的其他来源路径 JSON 数组。
// State 和计数从子库 dataset_info 中已提交的检查点同步。
type PCAPFileDBMetadata struct {
	gorm.Model
	DatasetID          string    `gorm:"unique_index;not null;size:64" json:"dataset_id"`
	DatabaseType       string    `gorm:"not null" json:"database_type"`
	DatabasePath       string    `gorm:"not null" json:"database_path"`
	SourcePath         string    `gorm:"index;not null" json:"source_path"`
	SourceAliases      string    `gorm:"type:text;not null" json:"source_aliases"`
	SourceSize         int64     `json:"source_size"`
	SourceModTimeNS    int64     `json:"source_mod_time_ns"`
	Fingerprint        string    `gorm:"index;not null" json:"fingerprint"`
	FingerprintMode    string    `json:"fingerprint_mode"`
	FingerprintVersion int       `json:"fingerprint_version"`
	FullSHA256         string    `gorm:"index" json:"full_sha256"`
	Format             string    `json:"format"`
	SchemaVersion      int       `json:"schema_version"`
	State              State     `gorm:"index;not null" json:"state"`
	CaptureRecordCount int64     `json:"capture_record_count"`
	SessionCount       int64     `json:"session_count"`
	StreamCount        int64     `json:"stream_count"`
	StreamChunkCount   int64     `json:"stream_chunk_count"`
	StreamDataSHA256   string    `json:"stream_data_sha256"`
	StreamDataSize     int64     `json:"stream_data_size"`
	StreamsIndexed     bool      `json:"streams_indexed"`
	PacketCount        int64     `json:"packet_count"`
	ProtocolCount      int64     `json:"protocol_count"`
	ProtocolDataSize   int64     `json:"protocol_data_size"`
	ProtocolDataSHA256 string    `json:"protocol_data_sha256"`
	BytesIndexed       int64     `json:"bytes_indexed"`
	ProtocolsIndexed   bool      `json:"protocols_indexed"`
	LastError          string    `gorm:"type:text" json:"last_error"`
	ValidatedAt        time.Time `json:"validated_at"`
}

func (*PCAPFileDBMetadata) TableName() string { return "pcapfile_db_metadata" }

func init() {
	schema.RegisterDatabaseSchema(schema.KEY_SCHEMA_PROFILE_DATABASE, &PCAPFileDBMetadata{})
}

// migratePCAPMetadata 仅迁移 profile 主库中的登记表和登记索引。
func migratePCAPMetadata(profile *gorm.DB) error {
	// Earlier review prototypes registered side-file paths as NOT NULL.
	// Remove those obsolete columns through GORM before inserting v4 catalog
	// rows; otherwise AutoMigrate would retain their old insert constraints.
	if profile.HasTable(&PCAPFileDBMetadata{}) {
		for _, column := range []string{"raw_path", "raw_mod_time_ns", "protocol_mod_time_ns"} {
			if profile.Dialect().HasColumn("pcapfile_db_metadata", column) {
				if err := profile.Model(&PCAPFileDBMetadata{}).DropColumn(column).Error; err != nil {
					return err
				}
			}
		}
	}
	if err := profile.AutoMigrate(&PCAPFileDBMetadata{}).Error; err != nil {
		return err
	}
	// 待导入记录的完整 SHA 尚未确定。部分唯一索引只约束已确认的内容，
	// 允许多个空 SHA 检查点；当前 GORM v1 的模型标签不支持 WHERE 索引。
	return profile.Exec("CREATE UNIQUE INDEX IF NOT EXISTS pcapfile_content_sha256 ON pcapfile_db_metadata(full_sha256) WHERE full_sha256 <> ''").Error
}
