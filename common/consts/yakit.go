package consts

import (
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/log"
)

var (
	projectDataBase *gorm.DB
	initOnce        = new(sync.Once)
	profileDatabase *gorm.DB
)

func SetGormProjectDatabase(d *gorm.DB) {
	log.Info("load gorm database connection")
	projectDataBase = d
}

func GetGormProfileDatabase() *gorm.DB {
	initYakitDatabase()
	return profileDatabase
}

func GetGormProjectDatabase() *gorm.DB {
	initYakitDatabase()
	return projectDataBase
}

func InitilizeYakitDatabase(projectDatabase string, profileDBName string) {
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
	initOnce.Do(func() {
		log.Debug("start to loading gorm project/profile database")
		var (
			err                 error = nil
			baseDir                   = GetDefaultYakitBaseDir()
			projectDatabaseName       = GetDefaultYakitProjectDatabase(baseDir)
			profileDatabaseName       = GetDefaultYakitPluginDatabase(baseDir)
		)

		/* 先创建插件数据库 */
		profileDatabase, err = createAndConfigDatabase(profileDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", profileDatabaseName, err)
		}
		/* 再创建项目数据库 */
		projectDataBase, err = createAndConfigDatabase(projectDatabaseName)
		if err != nil {
			log.Errorf("init plugin-db[%v] failed: %s", projectDatabaseName, err)
		}

		doDBPatch()
		doDBRiskPatch()
	})
}

func doDBPatch() {
	var err error
	if !projectDataBase.HasTable("http_flows") {
		return
	}
	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_source"
ON "http_flows" (
  "source_type" ASC
);`).Unscoped().Error
	if err != nil {
		log.Warnf("failed to add index on http_flows.source_type: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS "main"."idx_http_flows_tags"
ON "http_flows" (
  "tags" ASC
);`).Error
	if err != nil {
		log.Warnf("failed to add index on table: http_flows.tags: %v", err)
	}
}

func doDBRiskPatch() {
	if !projectDataBase.HasTable("risks") {
		return
	}
	err := projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_id ON risks(id);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.id: %v", err)
	}
	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_is_read ON risks(is_read);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.is_read: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_risk_type ON risks(risk_type);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.risk_type: %v", err)
	}

	err = projectDataBase.Exec(`CREATE INDEX IF NOT EXISTS main.idx_risks_ip ON risks(ip);`).Error
	if err != nil {
		log.Warnf("failed to add index on risks.ip: %v", err)
	}
}
