package consts

import (
	"runtime"
	"strings"
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
	projectReadDatabase       *gorm.DB
	profileDatabase           *gorm.DB
	debugProjectDatabase      = false
	debugProfileDatabase      = false
	initProjectReadDBOnce     = new(sync.Once)
	initProjectReadDBErr      error
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

// GetGormProjectDatabaseForRead returns a DB handle optimized for concurrent read queries.
//
// Background:
// - The main project DB is configured with MaxOpenConns=1 for write safety and to reduce "database is locked".
// - That makes read-heavy endpoints (like QueryHTTPFlows) queue behind writes and can become minutes-long under load.
// - A separate read-only handle avoids the connection pool bottleneck for reads.
func GetGormProjectDatabaseForRead() *gorm.DB {
	initYakitDatabase()

	initProjectReadDBOnce.Do(func() {
		baseDir := GetDefaultYakitBaseDir()
		projectDatabaseName := GetDefaultYakitProjectDatabase(baseDir)

		// Prefer a read-only SQLite connection when using SQLite drivers.
		// Fall back to the main DB on any failure.
		driver := DEFAULT_DRIVER
		var dsn string
		switch driver {
		case SQLite, SQLiteExtend:
			dsn = projectDatabaseName + "?mode=ro"
		default:
			dsn = projectDatabaseName
		}

		db, err := createAndConfigDatabase(dsn, driver)
		if err != nil {
			initProjectReadDBErr = err
			return
		}

		// Override pool settings for reads: allow concurrency.
		// WAL allows readers/writers concurrently; writes are still serialized by the write path.
		maxOpen := runtime.NumCPU()
		if maxOpen < 4 {
			maxOpen = 4
		}
		db.DB().SetMaxIdleConns(maxOpen)
		db.DB().SetMaxOpenConns(maxOpen)

		// Ensure read-only intent for SQLite (best-effort; ignore errors for non-SQLite dialects).
		_ = db.Exec("PRAGMA query_only = ON;").Error

		projectReadDatabase = db
	})

	if projectReadDatabase == nil {
		return GetGormProjectDatabase()
	}
	if debugProjectDatabase {
		return projectReadDatabase.Debug()
	}
	return projectReadDatabase
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
	// Drop the redundant single-column index in favor of the composite index below.
	_ = db.Exec(`DROP INDEX IF EXISTS "main"."idx_http_flows_source";`).Error

	// Drop the tags index to reduce write overhead; revisit if tag filtering becomes hot.
	_ = db.Exec(`DROP INDEX IF EXISTS "main"."idx_http_flows_tags";`).Error

	// Drop the standalone updated_at index; composite indexes cover current query paths.
	_ = db.Exec(`DROP INDEX IF EXISTS "main"."idx_http_flows_updated_at";`).Error

	// Frequent filters combined with time ordering.
	err = db.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_source_updated_at"
ON "http_flows" (
  "source_type" ASC,
  "updated_at" DESC
);`).Error
	if err != nil {
		log.Warnf("failed to add composite index on table: http_flows(source_type, updated_at): %v", err)
	}

	err = db.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_runtime_id_updated_at"
ON "http_flows" (
  "runtime_id" ASC,
  "updated_at" DESC
);`).Error
	if err != nil {
		log.Warnf("failed to add composite index on table: http_flows(runtime_id, updated_at): %v", err)
	}

	ensureHTTPFlowFTS(db)
}

func ensureHTTPFlowFTS(db *gorm.DB) {
	if !isSQLiteDialect(db) {
		return
	}

	// Fast check: if FTS5 module is not compiled in, skip and remove stale triggers to keep inserts working.
	if !supportsFTS5(db) {
		disableHTTPFlowFTS(db)
		return
	}

	// Trigram tokenizer makes MATCH a superset of LIKE for keywords >= 3 chars.
	// We still apply LIKE after MATCH to preserve exact semantics.
	if err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS "http_flows_fts" USING fts5(
	request,
	response,
	url,
	path,
	tags,
	remote_addr,
	content='http_flows',
	content_rowid='id',
	tokenize='trigram'
	);`).Error; err != nil {
		log.Warnf("failed to create http_flows_fts: %v", err)
		disableHTTPFlowFTS(db)
		return
	}

	if IsHTTPFlowFTSAsyncEnabled() {
		// Async FTS updates: drop triggers to avoid synchronous write amplification.
		dropHTTPFlowFTSTriggers(db)
	} else {
		// Keep FTS index in sync via triggers.
		triggers := []string{
			`CREATE TRIGGER IF NOT EXISTS "http_flows_fts_ai" AFTER INSERT ON "http_flows" BEGIN
  INSERT INTO "http_flows_fts"(rowid, request, response, url, path, tags, remote_addr)
  VALUES (new.id, new.request, new.response, new.url, new.path, new.tags, new.remote_addr);
END;`,
			`CREATE TRIGGER IF NOT EXISTS "http_flows_fts_ad" AFTER DELETE ON "http_flows" BEGIN
  INSERT INTO "http_flows_fts"("http_flows_fts", rowid, request, response, url, path, tags, remote_addr)
  VALUES ('delete', old.id, old.request, old.response, old.url, old.path, old.tags, old.remote_addr);
END;`,
			`CREATE TRIGGER IF NOT EXISTS "http_flows_fts_au" AFTER UPDATE ON "http_flows" BEGIN
  INSERT INTO "http_flows_fts"("http_flows_fts", rowid, request, response, url, path, tags, remote_addr)
  VALUES ('delete', old.id, old.request, old.response, old.url, old.path, old.tags, old.remote_addr);
  INSERT INTO "http_flows_fts"(rowid, request, response, url, path, tags, remote_addr)
  VALUES (new.id, new.request, new.response, new.url, new.path, new.tags, new.remote_addr);
END;`,
		}
		for _, stmt := range triggers {
			if err := db.Exec(stmt).Error; err != nil {
				log.Warnf("failed to create http_flows_fts trigger: %v", err)
				return
			}
		}
	}

	// Rebuild in background only if the FTS table is empty.
	go func() {
		tx := db.New()
		var count int
		if err := tx.Raw(`SELECT COUNT(*) FROM "http_flows_fts";`).Row().Scan(&count); err != nil {
			return
		}
		if count != 0 {
			return
		}
		if err := tx.Exec(`INSERT INTO "http_flows_fts"("http_flows_fts") VALUES('rebuild');`).Error; err != nil {
			log.Warnf("http_flows_fts rebuild failed: %v", err)
		}
	}()
}

// supportsFTS5 checks whether sqlite is built with the fts5 module.
// It uses a lightweight temp virtual table probe to avoid coupling to compile_options availability.
func supportsFTS5(db *gorm.DB) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	if !isSQLiteDialect(db) {
		return false
	}
	// Probe creation; if fts5 is missing this returns "no such module: fts5".
	if err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS temp.__fts5_probe USING fts5(content);`).Error; err != nil {
		return false
	}
	_ = db.Exec(`DROP TABLE IF EXISTS temp.__fts5_probe;`).Error
	return true
}

// disableHTTPFlowFTS removes triggers so inserts won't fail when fts5 module is unavailable.
func disableHTTPFlowFTS(db *gorm.DB) {
	dropHTTPFlowFTSTriggers(db)
	// Best effort: drop the virtual table if present (ignore errors if module is missing).
	_ = db.Exec(`DROP TABLE IF EXISTS "http_flows_fts";`).Error
	log.Infof("http_flows_fts disabled because sqlite fts5 module is unavailable")
}

func dropHTTPFlowFTSTriggers(db *gorm.DB) {
	stmts := []string{
		`DROP TRIGGER IF EXISTS "http_flows_fts_ai";`,
		`DROP TRIGGER IF EXISTS "http_flows_fts_ad";`,
		`DROP TRIGGER IF EXISTS "http_flows_fts_au";`,
	}
	for _, stmt := range stmts {
		_ = db.Exec(stmt).Error
	}
}

func isSQLiteDialect(db *gorm.DB) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	name := strings.ToLower(db.Dialect().GetName())
	return strings.Contains(name, "sqlite")
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
