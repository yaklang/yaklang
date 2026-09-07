package consts

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
)

const (
	YakitSQLiteProjectMaxOpenConnsEnv  = "YAKIT_SQLITE_PROJECT_MAX_OPEN_CONNS"
	YakitSQLiteProjectReadPoolConnsEnv = "YAKIT_SQLITE_PROJECT_READ_POOL_CONNS"
)

var (
	initYakitDatabaseRetError  error
	initYakitDatabaseOnce      = new(sync.Once)
	projectDataBase            *gorm.DB
	profileDatabase            *gorm.DB
	debugProjectDatabase       = false
	debugProfileDatabase       = false
	currentProjectDatabasePath string // 当前项目 SQLite 路径，在设置/初始化时写入，供慢 SQL 等在各创建处使用
	currentProfileDatabasePath string // 当前 profile SQLite 路径，在初始化时写入，供非 project 的慢 SQL 在各创建处使用

	projectDatabaseBinding    atomic.Pointer[ProjectDatabaseBinding]
	projectDatabaseGeneration atomic.Uint64
)

// ProjectDatabaseBinding is an immutable snapshot of the active project DB.
// Readers use one atomic load so a project switch cannot pair an old handle
// with a new path or generation.
type ProjectDatabaseBinding struct {
	Database     *gorm.DB
	ReadDatabase *gorm.DB
	Path         string
	Generation   uint64
}

func publishProjectDatabaseBinding(
	db *gorm.DB,
	readDB *gorm.DB,
	path string,
	forceNewGeneration bool,
) ProjectDatabaseBinding {
	if !forceNewGeneration {
		if current := projectDatabaseBinding.Load(); current != nil && current.Database == db &&
			current.ReadDatabase == readDB && current.Path == path {
			return *current
		}
	}
	binding := &ProjectDatabaseBinding{
		Database:     db,
		ReadDatabase: readDB,
		Path:         path,
		Generation:   projectDatabaseGeneration.Add(1),
	}
	// Queued writes and in-flight queries can retain the previous generation.
	// Its writer already follows this deferred lifecycle, so the optional reader
	// must not be closed eagerly during an atomic project rebind either.
	projectDatabaseBinding.Store(binding)
	return *binding
}

func DebugProjectDatabase() {
	debugProjectDatabase = true
}

func DebugProfileDatabase() {
	debugProfileDatabase = true
}

func CreateProjectDatabase(path string) (*gorm.DB, error) {
	options, err := projectDatabaseOpenOptions()
	if err != nil {
		return nil, err
	}
	if _, err := projectDatabaseReadPoolConns(); err != nil {
		return nil, err
	}
	db, err := createAndConfigDatabaseWithOptions(path, options)
	if err != nil {
		return nil, err
	}
	if options.sqliteMaxOpenConns > 1 {
		log.Infof(
			"project SQLite read concurrency enabled: max_open_connections=%d cache=private",
			options.sqliteMaxOpenConns,
		)
	}
	TuneSQLiteByDatabaseFileSize(db, path)
	schema.AutoMigrate(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	schema.ApplyPatches(db, schema.KEY_SCHEMA_YAKIT_DATABASE)
	return db, nil
}

// CreateProjectDatabaseReadOnly opens the optional query-only pool used by
// QueryHTTPFlows experiments. A nil result means the feature is disabled.
func CreateProjectDatabaseReadOnly(path string) (*gorm.DB, error) {
	maxOpenConns, err := projectDatabaseReadPoolConns()
	if err != nil || maxOpenConns == 0 {
		return nil, err
	}
	db, err := createSQLiteReadOnlyDatabase(path, maxOpenConns)
	if err != nil {
		return nil, err
	}
	log.Infof(
		"project SQLite dedicated read pool enabled: max_open_connections=%d mode=ro query_only=1 cache=private",
		maxOpenConns,
	)
	return db, nil
}

func projectDatabaseOpenOptions() (databaseOpenOptions, error) {
	options := defaultDatabaseOpenOptions()
	raw := strings.TrimSpace(os.Getenv(YakitSQLiteProjectMaxOpenConnsEnv))
	if raw == "" {
		return options, nil
	}
	maxOpenConns, err := strconv.Atoi(raw)
	if err != nil || maxOpenConns < 1 || maxOpenConns > 8 {
		return options, utils.Errorf(
			"%s must be an integer between 1 and 8, got %q",
			YakitSQLiteProjectMaxOpenConnsEnv,
			raw,
		)
	}
	options.sqliteMaxOpenConns = maxOpenConns
	options.sqlitePrivateCache = maxOpenConns > 1
	return options, nil
}

func projectDatabaseReadPoolConns() (int, error) {
	raw := strings.TrimSpace(os.Getenv(YakitSQLiteProjectReadPoolConnsEnv))
	if raw == "" {
		return 0, nil
	}
	maxOpenConns, err := strconv.Atoi(raw)
	if err != nil || maxOpenConns < 0 || maxOpenConns > 4 {
		return 0, utils.Errorf(
			"%s must be an integer between 0 and 4, got %q",
			YakitSQLiteProjectReadPoolConnsEnv,
			raw,
		)
	}
	return maxOpenConns, nil
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

func isSQLiteRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked") ||
		strings.Contains(msg, "being used by another") ||
		strings.Contains(msg, "unable to open database file")
}

func SetGormProjectDatabase(path string) error {
	var lastErr error
	for i := 0; i < 8; i++ {
		lastErr = setGormProjectDatabaseOnce(path)
		if lastErr == nil {
			return nil
		}
		if !isSQLiteRetryableErr(lastErr) {
			return lastErr
		}
		shift := i
		if shift > 5 {
			shift = 5
		}
		time.Sleep(time.Duration(50*(1<<shift)) * time.Millisecond)
	}
	return lastErr
}

func setGormProjectDatabaseOnce(path string) error {
	d, err := CreateProjectDatabase(path)
	if err != nil {
		return err
	}
	readDB, err := CreateProjectDatabaseReadOnly(path)
	if err != nil {
		_ = d.Close()
		return err
	}
	BindProjectDatabaseWithReader(d, readDB, path)
	return nil
}

// BindProjectDatabase registers an already-open project DB as the global project handle.
// Used by grpc test servers so DBSaveAsyncChannel and Server.GetProjectDatabase() share one SQLite file.
func BindProjectDatabase(db *gorm.DB, path string) {
	readDB, err := CreateProjectDatabaseReadOnly(path)
	if err != nil {
		log.Warnf("open project SQLite dedicated read pool failed, falling back to writer handle: %v", err)
	}
	BindProjectDatabaseWithReader(db, readDB, path)
}

// BindProjectDatabaseWithReader registers an already-open writer and optional
// query-only reader as one coherent project generation.
func BindProjectDatabaseWithReader(db *gorm.DB, readDB *gorm.DB, path string) {
	projectDataBase = db
	currentProjectDatabasePath = path
	schema.SetGormProjectDatabase(db)
	publishProjectDatabaseBinding(db, readDB, path, true)
}

// BindProfileDatabase registers an already-open profile DB as the global profile handle.
// Used by grpc test servers so lowhttp MITM replacer hooks and plugin DB access share one SQLite file.
func BindProfileDatabase(db *gorm.DB, path string) {
	profileDatabase = db
	currentProfileDatabasePath = path
	schema.SetGormProfileDatabase(db)
}

// CloseProfileDatabase closes the global profile DB handle and clears the
// binding so the underlying SQLite file can be replaced on disk. Used by the
// plugin store import flow to atomically swap the local plugin database.
func CloseProfileDatabase() {
	if profileDatabase != nil {
		if sqlDB := profileDatabase.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
	}
	profileDatabase = nil
	schema.SetGormProfileDatabase(nil)
}

func GetGormProfileDatabase() *gorm.DB {
	if profileDatabase != nil {
		if debugProfileDatabase {
			return profileDatabase.Debug()
		}
		return profileDatabase
	}
	initYakitDatabase()
	if debugProfileDatabase {
		return profileDatabase.Debug()
	}
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	if binding := projectDatabaseBinding.Load(); binding != nil && binding.Database != nil {
		if debugProjectDatabase {
			return binding.Database.Debug()
		}
		return binding.Database
	}
	if projectDataBase != nil {
		if debugProjectDatabase {
			return projectDataBase.Debug()
		}
		return projectDataBase
	}
	initYakitDatabase()
	if debugProjectDatabase {
		return projectDataBase.Debug()
	}
	return projectDataBase
}

// GetCurrentProjectDatabasePath 返回当前项目 SQLite 数据库文件路径；仅在慢 SQL 等使用 project DB 的创建处调用
func GetCurrentProjectDatabasePath() string {
	if binding := projectDatabaseBinding.Load(); binding != nil && binding.Path != "" {
		return binding.Path
	}
	if currentProjectDatabasePath != "" {
		return currentProjectDatabasePath
	}
	initYakitDatabase()
	if currentProjectDatabasePath != "" {
		return currentProjectDatabasePath
	}
	return GetDefaultYakitProjectDatabase(GetDefaultYakitBaseDir())
}

// CaptureProjectDatabaseBinding returns a coherent DB/path/generation tuple.
// Generation changes on every explicit bind, including reopening the same path.
func CaptureProjectDatabaseBinding() ProjectDatabaseBinding {
	if binding := projectDatabaseBinding.Load(); binding != nil && binding.Database != nil {
		return *binding
	}
	_ = initYakitDatabase()
	if binding := projectDatabaseBinding.Load(); binding != nil {
		return *binding
	}
	return publishProjectDatabaseBinding(projectDataBase, nil, currentProjectDatabasePath, false)
}

// AdvanceProjectDatabaseGeneration marks the current project database as a new
// logical incarnation without reopening its handles. Destructive table
// recreation uses this so cursors and process-local streams from the previous
// incarnation cannot hide rows whose SQLite IDs started over.
//
// The expected generation makes the operation conditional: if a project switch
// raced with the caller, the new project binding is left untouched.
func AdvanceProjectDatabaseGeneration(expectedGeneration uint64) (ProjectDatabaseBinding, bool) {
	for {
		current := projectDatabaseBinding.Load()
		if current == nil || current.Database == nil || current.Generation != expectedGeneration {
			if current == nil {
				return ProjectDatabaseBinding{}, false
			}
			return *current, false
		}
		next := &ProjectDatabaseBinding{
			Database:     current.Database,
			ReadDatabase: current.ReadDatabase,
			Path:         current.Path,
			Generation:   projectDatabaseGeneration.Add(1),
		}
		if projectDatabaseBinding.CompareAndSwap(current, next) {
			return *next, true
		}
	}
}

// GetCurrentProfileDatabasePath 返回当前 profile SQLite 数据库文件路径；仅在慢 SQL 等使用 profile DB 的创建处调用
func GetCurrentProfileDatabasePath() string {
	if currentProfileDatabasePath != "" {
		return currentProfileDatabasePath
	}
	initYakitDatabase()
	if currentProfileDatabasePath != "" {
		return currentProfileDatabasePath
	}
	return GetDefaultYakitPluginDatabase(GetDefaultYakitBaseDir())
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
	GetDefaultYakitProjectsDir()         // yakit-projects/projects
	GetDefaultYakitPayloadsDir()         // yakit-projects/payloads
	GetDefaultYakitEngineDir()           // yakit-projects/yak-engine
	GetDefaultYakitPprofDir()            // yakit-projects/pprof-log
	GetDefaultYakitBaseTempDir()         // yakit-projects/temp
	GetDefaultAISkillsDir()              // yakit-projects/ai-skills
	GetDefaultYakitOpenAPIDocumentsDir() // yakit-projects/openapi-documents

	utils.RegisterTempFileOpener(func(name string) (*os.File, error) {
		dir := GetDefaultYakitBaseTempDir()
		if !utils.IsDir(dir) {
			_ = os.MkdirAll(dir, 0o755)
		}
		return os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	})

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
		if profileDatabase == nil {
			profileDatabase, err = CreateProfileDatabase(profileDatabaseName)
			if err != nil {
				err = utils.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
				log.Errorf("%s", err)
				initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
			}
			currentProfileDatabasePath = profileDatabaseName
		}
		schema.SetGormProfileDatabase(profileDatabase)

		/* 再创建项目数据库 */
		var projectReadDatabase *gorm.DB
		if projectDataBase == nil {
			projectDataBase, err = CreateProjectDatabase(projectDatabaseName)
			if err != nil {
				err = utils.Errorf("init project-db[%v] failed: %s", projectDatabaseName, err)
				log.Errorf("%s", err)
				initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
			}
			currentProjectDatabasePath = projectDatabaseName
		}
		if projectDataBase != nil {
			projectReadDatabase, err = CreateProjectDatabaseReadOnly(currentProjectDatabasePath)
			if err != nil {
				err = utils.Errorf("init project read-db[%v] failed: %s", currentProjectDatabasePath, err)
				log.Errorf("%s", err)
				initYakitDatabaseRetError = utils.JoinErrors(initYakitDatabaseRetError, err)
			}
		}
		schema.SetGormProjectDatabase(projectDataBase)
		publishProjectDatabaseBinding(projectDataBase, projectReadDatabase, currentProjectDatabasePath, false)

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
