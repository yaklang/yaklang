package consts

import (
	"sync"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
)

var (
	initYakitDatabaseRetError error
	initYakitDatabaseOnce     = new(sync.Once)
	projectDataBase           *gorm.DB
	profileDatabase           *gorm.DB
	debugProjectDatabase      = false
	debugProfileDatabase      = false
)

func DebugProjectDatabase() {
	debugProjectDatabase = true
}

func DebugProfileDatabase() {
	debugProfileDatabase = true
}

func CreateProjectDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	return db, nil
}

func CreateProfileDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_PROFILE_DATABASE)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_PROFILE_DATABASE)
	return db, nil
}

func SetGormProjectDatabase(path string) error {
	d, err := CreateProjectDatabase(path)
	if err != nil {
		return err
	}
	projectDataBase = d
	schema.AutoMigrate(d, schema.KEY_SCHEMA_YAKIT_DATABASE)
	schema.SetGormProjectDatabase(d)
	return nil
}

func GetGormProfileDatabase() *gorm.DB {
	initYakitDatabase()
	if debugProfileDatabase {
		return profileDatabase.Debug()
	}
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	initYakitDatabase()
	if debugProjectDatabase {
		return projectDataBase.Debug()
	}
	return projectDataBase
}

func InitializeYakitDatabase(projectDB string, profileDB string, ssaDB string) error {

	initializeYakitDirectories()

	// profile
	profileDBName := GetProfileDatabaseNameFromEnv()
	if profileDB != "" {
		profileDBName = profileDB
	}
	SetDefaultYakitProfileDatabaseName(profileDBName)

	// project
	projectName := GetProjectDatabaseNameFromEnv()
	if projectDB != "" {
		projectName = projectDB
	}
	SetDefaultYakitProjectDatabaseName(projectName)

	// ssa check env
	ssaProjectDatabaseRaw := GetSSADatabaseInfoFromEnv()
	if ssaDB != "" {
		ssaProjectDatabaseRaw = ssaDB
	}
	SetSSADatabaseInfo(ssaProjectDatabaseRaw)

	return initYakitDatabase()
}

// initializeYakitDirectories 确保所有必要的Yakit目录在项目初始化时就被创建
func initializeYakitDirectories() {
	GetDefaultYakitProjectsDir() // yakit-projects/projects
	GetDefaultYakitPayloadsDir() // yakit-projects/payloads
	GetDefaultYakitEngineDir()   // yakit-projects/yak-engine
	GetDefaultYakitPprofDir()    // yakit-projects/pprof-log
	GetDefaultYakitBaseTempDir() // yakit-projects/temp

	log.Debug("yakit directories initialized")
}

func initYakitDatabase() error {
	initYakitDatabaseOnce.Do(func() {
		initYakitDatabaseRetError = nil
		log.Debug("start to loading gorm project/profile database")
		var (
			err                 error
			baseDir             = GetDefaultYakitBaseDir()
			projectDatabaseName = GetDefaultYakitProjectDatabase(baseDir)
			profileDatabaseName = GetDefaultYakitPluginDatabase(baseDir)
		)

		/* 先创建插件数据库 */
		profileDatabase, err = CreateProfileDatabase(profileDatabaseName)
		if err != nil {
			err = utils.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
			log.Errorf("%s", err)
			initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
		}
		schema.SetGormProfileDatabase(profileDatabase)

		/* 再创建项目数据库 */
		projectDataBase, err = CreateProjectDatabase(projectDatabaseName)
		if err != nil {
			err = utils.Errorf("init project-db[%v] failed: %s", projectDatabaseName, err)
			log.Errorf("%s", err)
			initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
		}
		schema.SetGormProjectDatabase(projectDataBase)

		/* 创建SSA数据库 */
		ssaDatabaseDialect, ssaDatabaseRaw := GetSSADataBaseInfo()
		ssaprojectDatabase, err := CreateSSAProjectDatabase(ssaDatabaseDialect, ssaDatabaseRaw)
		if err != nil {
			err = utils.Errorf("init ssa-db[%s %s] failed: %s", ssaDatabaseRaw, ssaDatabaseDialect, err)
			log.Errorf("%s", err)
			initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
		}
		schema.SetDefaultSSADatabase(ssaprojectDatabase)
		SetGormSSAProjectDatabase(ssaprojectDatabase)
	})
	return initYakitDatabaseRetError
}

func init() {
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_YAKIT_DATABASE, doHTTPFlowPatch)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_YAKIT_DATABASE, doDBRiskPatch)
	schema.RegisterDatabasePatch(schema.KEY_SCHEMA_YAKIT_DATABASE, doAIEventPatch)
}

func doHTTPFlowPatch(db *gorm.DB) {
	var err error
	if !db.HasTable("http_flows") {
		return
	}
	err = db.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_source"
ON "http_flows" (
  "source_type" ASC
);`).Unscoped().Error
	if err != nil {
		log.Warnf("failed to add index on http_flows.source_type: %v", err)
	}

	err = db.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_tags"
ON "http_flows" (
  "tags" ASC
);`).Error
	if err != nil {
		log.Warnf("failed to add index on table: http_flows.tags: %v", err)
	}
}

func doDBRiskPatch(db *gorm.DB) {
	if !db.HasTable("risks") {
		return
	}
	err := db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_id ON risks(id);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.id: %v", err)
	}
	err = db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_is_read ON risks(is_read);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.is_read: %v", err)
	}

	err = db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_risk_type ON risks(risk_type);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.risk_type: %v", err)
	}

	err = db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_ip ON risks(ip);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.ip: %v", err)
	}
}

func doAIEventPatch(db *gorm.DB) {
	// add indexes for ai_output_events table to improve save/query performance
	if db.HasTable("ai_output_events") {
		indexQueries := []struct {
			name  string
			query string
		}{
			{"idx_ai_output_events_coordinator_id", `CREATE INDEX IF NOT EXISTS "idx_ai_output_events_coordinator_id" ON "ai_output_events" ("coordinator_id");`},
			{"idx_ai_output_events_event_uuid", `CREATE INDEX IF NOT EXISTS "idx_ai_output_events_event_uuid" ON "ai_output_events" ("event_uuid");`},
			{"idx_ai_output_events_task_index", `CREATE INDEX IF NOT EXISTS "idx_ai_output_events_task_index" ON "ai_output_events" ("task_index");`},
			{"idx_ai_output_events_task_uuid", `CREATE INDEX IF NOT EXISTS "idx_ai_output_events_task_uuid" ON "ai_output_events" ("task_uuid");`},
			{"idx_ai_output_events_call_tool_id", `CREATE INDEX IF NOT EXISTS "idx_ai_output_events_call_tool_id" ON "ai_output_events" ("call_tool_id");`},
		}
		for _, idx := range indexQueries {
			if err := db.Exec(idx.query).Error; err != nil {
				log.Warnf("failed to add index %s on ai_output_events: %v", idx.name, err)
			}
		}
	}

	// add indexes for ai_processes table
	if db.HasTable("ai_processes") {
		indexQueries := []struct {
			name  string
			query string
		}{
			{"idx_ai_processes_process_type", `CREATE INDEX IF NOT EXISTS "idx_ai_processes_process_type" ON "ai_processes" ("process_type");`},
			{"idx_ai_processes_process_id", `CREATE INDEX IF NOT EXISTS "idx_ai_processes_process_id" ON "ai_processes" ("process_id");`},
		}
		for _, idx := range indexQueries {
			if err := db.Exec(idx.query).Error; err != nil {
				log.Warnf("failed to add index %s on ai_processes: %v", idx.name, err)
			}
		}
	}
}
