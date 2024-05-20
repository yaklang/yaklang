package consts

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	gormCVEDatabase     *gorm.DB
	gormCVEDescDatabase *gorm.DB
)

func GetCVEDatabasePath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve.db")
}

func GetCVEDescriptionDatabasePath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve-description.db")
}

func GetCVEDescriptionDatabaseGzipPath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve-description.db.gzip")
}

func GetCVEDatabaseGzipPath() string {
	return filepath.Join(GetDefaultYakitBaseDir(), "default-cve.db.gzip")
}

func SetGormCVEDatabase(db *gorm.DB) {
	if gormCVEDatabase == nil {
		gormCVEDatabase = db
		return
	}
	gormCVEDatabase.Close()
	gormCVEDatabase = db
	return
}

func GetGormCVEDatabase() *gorm.DB {
	if gormCVEDatabase == nil {
		var err error
		gormCVEDatabase, err = InitializeCVEDatabase()
		if err != nil {
			log.Debugf("initialize cve db failed: %s", err)
		}
	}
	return gormCVEDatabase
}

func GetGormCVEDescriptionDatabase() *gorm.DB {
	if gormCVEDescDatabase == nil {
		var err error
		gormCVEDescDatabase, err = InitializeCVEDescriptionDatabase()
		if err != nil {
			log.Debugf("initialize cve db failed: %s", err)
		}
	}
	return gormCVEDescDatabase
}

func CreateCVEDescriptionDatabase(path string) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path)
	if err != nil {
		return nil, err
	}
	schema.AutoMigrate(db, schema.KEY_SCHEMA_CVE_DESCRIPTION_DATABASE)
	return db, nil
}

func CreateCVEDatabase(path string, patch ...bool) (*gorm.DB, error) {
	db, err := createAndConfigDatabase(path)
	if err != nil {
		return nil, err
	}

	schema.AutoMigrate(db, schema.KEY_SCHEMA_CVE_DATABASE)
	shouldPatch := true
	if len(patch) > 0 {
		shouldPatch = patch[0]
	}
	if shouldPatch {
		doCVEPatch(db)
	}

	return db, nil
}

func InitializeCVEDescriptionDatabase() (*gorm.DB, error) {
	log.Info("start to initialize cve desc db")
	cveDescDb := GetCVEDescriptionDatabasePath()
	cveDescGzip := GetCVEDescriptionDatabaseGzipPath()
	ret := utils.GetFirstExistedFile(cveDescDb, cveDescGzip)
	log.Infof("init CVE Description database: found first existed file: %s", ret)
	if ret == cveDescGzip {
		log.Infof("init CVE Description database: start to un-gzip %v", cveDescGzip)
		fp, err := os.Open(cveDescGzip)
		if err != nil {
			return nil, err
		}
		defer fp.Close()
		dbFp, err := os.OpenFile(cveDescDb, os.O_RDWR|os.O_CREATE, 0o666)
		if err != nil {
			return nil, err
		}
		defer dbFp.Close()

		gr, err := gzip.NewReader(fp)
		if err != nil {
			return nil, utils.Errorf("un-gzip for %v failed: %s", cveDescDb, err)
		}
		io.Copy(dbFp, gr)
		log.Infof("init CVE Description database: finished to create: %s", cveDescGzip)
	}

	if ret == "" {
		return nil, utils.Error("no cve description db found")
	}

	db, err := CreateCVEDescriptionDatabase(cveDescDb)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func InitializeCVEDatabase() (*gorm.DB, error) {
	cveDatabase := GetCVEDatabasePath()
	cveDatabaseEncrypted := GetCVEDatabaseGzipPath()

	// 自动化解压
	if utils.GetFirstExistedFile(cveDatabase) == "" && utils.GetFirstExistedFile(cveDatabaseEncrypted) != "" {
		fp, err := os.Open(cveDatabaseEncrypted)
		if err != nil {
			return nil, err
		}
		dbFp, err := os.OpenFile(cveDatabase, os.O_RDWR|os.O_CREATE, 0o666)
		if err != nil {
			fp.Close()
			return nil, err
		}
		gr, err := gzip.NewReader(fp)
		if err != nil {
			fp.Close()
			dbFp.Close()
			return nil, utils.Errorf("un-gzip for %v failed: %s", cveDatabase, err)
		}
		io.Copy(dbFp, gr)
		dbFp.Close()
		fp.Close()
	}

	var err error

	gormCVEDatabase, err = CreateCVEDatabase(cveDatabase, false)
	if err != nil {
		return nil, utils.Errorf(`cve database[%v] failed: %s`, cveDatabase, err)
	}
	// issue #725 这一步要在添加索引之前，否则会从添加索引的 return 语句中返回
	// 如果没有表就删除 open 产生的文件
	if !gormCVEDatabase.HasTable("cves") {
		gormCVEDatabase.Close()
		gormCVEDatabase = nil
		err := DeleteDatabaseFile(cveDatabase)
		if err != nil {
			return nil, utils.Errorf("remove [%s] failed: %v", cveDatabase, err)
		}
		return nil, utils.Errorf("cve database failed: %s", "empty")
	}
	doCVEPatch(gormCVEDatabase)

	return gormCVEDatabase, nil
}

func doCVEPatch(db *gorm.DB) {
	err := db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_cves_cve ON cves(CVE);`).Error
	if err != nil {
		log.Warnf("failed to add index on cves.CVE: %v", err)
	}

	err = db.Exec(`CREATE INDEX IF NOT EXISTS main.idx_cwes_id_str ON cwes(id_str);`).Error
	if err != nil {
		log.Warnf("failed to add index on cwes.id_str: %v", err)
	}
}
