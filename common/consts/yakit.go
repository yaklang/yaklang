package consts

import (
	"sync"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
)

var (
	initYakitDatabaseOnce = new(sync.Once)
	projectDataBase       *gorm.DB
	profileDatabase       *gorm.DB
	debugProjectDatabase  = false
	debugProfileDatabase  = false
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
	doHTTPFlowPatch(db)
	doDBRiskPatch(db)
	return db, nil
}

func CreateProfileDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_PROFILE_DATABASE)
	return db, nil
}

func SetGormProjectDatabase(d *gorm.DB) {
	log.Info("load gorm database connection")
	projectDataBase = d
	schema.SetGormProjectDatabase(d)
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

func InitializeYakitDatabase(projectDatabase string, profileDBName string) {
	projectName := GetProjectDatabaseNameFromEnv()
	if projectDatabase != "" {
		projectName = projectDatabase
	}
	profileDatabase := GetProfileDatabaseNameFromEnv()
	if profileDBName != "" {
		profileDatabase = profileDBName
	}
	SetDefaultYakitProjectDatabaseName(projectName)
	SetDefaultYakitProfileDatabaseName(profileDatabase)
	initYakitDatabase()
}

func initYakitDatabase() {
	initYakitDatabaseOnce.Do(func() {
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
			log.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
		}
		/* 再创建项目数据库 */
		projectDataBase, err = CreateProjectDatabase(projectDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", projectDatabaseName, err)
		}

		schema.SetGormProjectDatabase(projectDataBase)
		schema.SetGormProfileDatabase(profileDatabase)
	})
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
