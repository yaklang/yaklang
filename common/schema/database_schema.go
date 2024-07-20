package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
)

var (
	projectDataBase *gorm.DB
	profileDatabase *gorm.DB
)

func init() {
	RegisterDatabaseSchema(KEY_SCHEMA_YAKIT_DATABASE, ProjectTables...)
	RegisterDatabaseSchema(KEY_SCHEMA_PROFILE_DATABASE, ProfileTables...)
}

const (
	KEY_SCHEMA_YAKIT_DATABASE uint8 = iota
	KEY_SCHEMA_PROFILE_DATABASE
	KEY_SCHEMA_CVE_DATABASE
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE
	KEY_SCHEMA_VULINBOX_DATABASE
	KEY_SCHEMA_SSA_DATABASE
	KEY_SCHEMA_SYNTAXFLOW_RULE
)

// ProfileTables 这些表是独立与项目之外的，每一个用户的数据都不一样
var ProfileTables = []interface{}{
	&YakScript{}, &Payload{}, &MenuItem{},
	&GeneralStorage{}, &MarkdownDoc{},
	&Project{},
	&NavigationBar{}, &NaslScript{},
	&WebFuzzerLabel{},
	&PluginGroup{},
	&CodecFlow{},
}

var databaseSchemas = map[uint8][]any{
	KEY_SCHEMA_YAKIT_DATABASE:           nil,
	KEY_SCHEMA_PROFILE_DATABASE:         nil,
	KEY_SCHEMA_CVE_DATABASE:             nil,
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE: nil,
	KEY_SCHEMA_VULINBOX_DATABASE:        nil,
	KEY_SCHEMA_SSA_DATABASE:             nil,
	KEY_SCHEMA_SYNTAXFLOW_RULE:          nil,
}

// ProjectTables 这些表是和项目关联的，导出项目可以直接复制给用户
var ProjectTables = []interface{}{
	&WebsocketFlow{},
	&HTTPFlow{}, &ExecHistory{},
	&ExtractedData{},
	&Port{},
	&Domain{}, &Host{},
	&MarkdownDoc{}, &ExecResult{},
	&Risk{}, &WebFuzzerTask{}, &WebFuzzerResponse{},
	&ReportRecord{}, &ScreenRecorder{},
	&ProjectGeneralStorage{},
	// rss
	&Briefing{}, &RssFeed{}, &WebShell{},
	// &assets.SubscriptionSource{},
	&AliveHost{},

	// traffic
	&TrafficSession{}, &TrafficPacket{}, &TrafficTCPReassembledFrame{},

	// HybridScan
	&HybridScanTask{},

	// Progress
	&Progress{},
}

func RegisterDatabaseSchema(key uint8, schema ...any) {
	if _, ok := databaseSchemas[key]; !ok {
		panic("Database schema key invalid")
	}

	databaseSchemas[key] = lo.Uniq(append(databaseSchemas[key], schema...))
}

func AutoMigrate(db *gorm.DB, key uint8) {
	if schemas, ok := databaseSchemas[key]; ok {
		if len(schemas) == 0 {
			panic("Database schema is empty")
		}
		db.AutoMigrate(schemas...)
	} else {
		panic("Database schema key invalid")
	}
}

// set from consts package
func SetGormProjectDatabase(d *gorm.DB) {
	projectDataBase = d
}

// set from consts package
func SetGormProfileDatabase(d *gorm.DB) {
	profileDatabase = d
}

func GetGormProfileDatabase() *gorm.DB {
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	return projectDataBase
}
