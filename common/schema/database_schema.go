package schema

import (
	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
)

var (
	projectDataBase    *gorm.DB
	profileDatabase    *gorm.DB
	defaultSSADataBase *gorm.DB
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
)

func KeySchemaToName(i uint8) string {
	switch i {
	case KEY_SCHEMA_YAKIT_DATABASE:
		return "KEY_SCHEMA_YAKIT_DATABASE"
	case KEY_SCHEMA_PROFILE_DATABASE:
		return "KEY_SCHEMA_PROFILE_DATABASE"
	case KEY_SCHEMA_CVE_DATABASE:
		return "KEY_SCHEMA_CVE_DATABASE"
	case KEY_SCHEMA_CVE_DESCRIPTION_DATABASE:
		return "KEY_SCHEMA_CVE_DESCRIPTION_DATABASE"
	case KEY_SCHEMA_VULINBOX_DATABASE:
		return "KEY_SCHEMA_VULINBOX_DATABASE"
	case KEY_SCHEMA_SSA_DATABASE:
		return "KEY_SCHEMA_SSA_DATABASE"
	default:
		return "KEY_SCHEMA_Unknown"
	}
}

// ProfileTables 这些表是独立与项目之外的，每一个用户的数据都不一样
var ProfileTables = []interface{}{
	&YakScript{}, &Payload{}, &MenuItem{},
	&GeneralStorage{}, &MarkdownDoc{},
	&Project{},
	&NavigationBar{}, &NaslScript{},
	&WebFuzzerLabel{},
	&PluginGroup{},
	&CodecFlow{},
	//general rule
	&GeneralRule{},
	&GeneralRuleGroup{},
	//syntaxFlow rule
	&SyntaxFlowRule{},
	&SyntaxFlowGroup{},
	&PluginEnv{},
	&HotPatchTemplate{},
	&AIForge{},

	&AiProvider{},   // for aibalance
	&AiApiKeys{},    // for aibalance
	&LoginSession{}, // for aibalance
	&AIYakTool{},

	&Snippets{}, // Snippets
	// SSA Projects Config Info
	&SSAProject{},
}

var databaseSchemas = map[uint8][]any{
	KEY_SCHEMA_YAKIT_DATABASE:           nil,
	KEY_SCHEMA_PROFILE_DATABASE:         nil,
	KEY_SCHEMA_CVE_DATABASE:             nil,
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE: nil,
	KEY_SCHEMA_VULINBOX_DATABASE:        nil,
	KEY_SCHEMA_SSA_DATABASE:             nil,
}

var databasePatches = map[uint8][]func(db *gorm.DB){
	KEY_SCHEMA_YAKIT_DATABASE:           nil,
	KEY_SCHEMA_PROFILE_DATABASE:         nil,
	KEY_SCHEMA_CVE_DATABASE:             nil,
	KEY_SCHEMA_CVE_DESCRIPTION_DATABASE: nil,
	KEY_SCHEMA_VULINBOX_DATABASE:        nil,
	KEY_SCHEMA_SSA_DATABASE:             nil,
}

// ProjectTables 这些表是和项目关联的，导出项目可以直接复制给用户
var ProjectTables = []interface{}{
	&WebsocketFlow{},
	&HTTPFlow{}, &ExecHistory{}, &AnalyzedHTTPFlow{},
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
	// WebFuzzer Config
	&WebFuzzerConfig{},

	// Note
	&Note{},

	// AI
	&AIAgentRuntime{},
	&AiCheckpoint{},
	&AiOutputEvent{},
	&AiProcess{},
	&AIMemoryEntity{},
	&AIMemoryCollection{},

	// project level vector collection
	&VectorStoreCollection{},
	&VectorStoreDocument{},
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
			log.Errorf("Database schema [%v] is empty", KeySchemaToName(key))
			return
		}
		db.AutoMigrate(schemas...)
	} else {
		log.Errorf("Database schema key: %v is %v", key, KeySchemaToName(key))
	}
}

func RegisterDatabasePatch(key uint8, patch func(db *gorm.DB)) {
	if _, ok := databasePatches[key]; !ok {
		panic("Database schema key invalid")
	}

	databasePatches[key] = append(databasePatches[key], patch)
}

func ApplyPatches(db *gorm.DB, key uint8) {
	if patches, ok := databasePatches[key]; ok {
		for _, patch := range patches {
			if patch == nil {
				continue
			}
			patch(db)
		}
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

func SetDefaultSSADatabase(d *gorm.DB) {
	defaultSSADataBase = d
}

func GetGormProfileDatabase() *gorm.DB {
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	return projectDataBase
}

func GetDefaultSSADatabase() *gorm.DB {
	return defaultSSADataBase
}
