package consts

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/cve/cveresources"
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

func InitializeCVEDescriptionDatabase() (*gorm.DB, error) {
	log.Info("start to initialize cve desc db")
	cveDescDb := GetCVEDescriptionDatabasePath()
	cveDescGzip := GetCVEDescriptionDatabaseGzipPath()
	ret := utils.GetFirstExistedFile(cveDescDb, cveDescGzip)
	log.Infof("found first existed file: %s", ret)
	if ret == cveDescGzip {
		log.Infof("start to un-gzip %v", cveDescGzip)
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
		log.Infof("finished to create: %s", cveDescGzip)
	}
	if ret == "" {
		return nil, utils.Error("no cve description db found")
	}
	return gorm.Open("sqlite3", cveDescDb)
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

	gormCVEDatabase, err = createAndConfigDatabase(cveDatabase)
	if err != nil {
		return nil, utils.Errorf(`cve database[%v] failed: %s`, cveDatabase, err)
	}
	// issue #725 这一步要在添加索引之前，否则会从添加索引的 return 语句中返回
	// 如果没有表就删除 open 产生的文件
	if !gormCVEDatabase.HasTable(&cveresources.CVE{}) {
		gormCVEDatabase.Close()
		gormCVEDatabase = nil
		err := os.RemoveAll(cveDatabase)
		if err != nil {
			return nil, utils.Errorf("remove [%s] failed: %v", cveDatabase, err)
		}
		return nil, utils.Errorf("cve database failed: %s", "empty")
	}
	var count int
	_ = gormCVEDatabase.DB().QueryRow("PRAGMA index_info(idx_cves_cve)").Scan(&count)
	// 如果没有索引就添加
	if count == 0 {
		err = gormCVEDatabase.Model(&cveresources.CVE{}).AddIndex("idx_cves_cve", "CVE").Error
		if err != nil {
			return nil, utils.Errorf(`add index  failed: %s`, err)
		}
	}
	var cweCount int
	err = gormCVEDatabase.Model(&cveresources.CWE{}).AddIndex("idx_cwes_id_str", "IdStr").Error
	if cweCount == 0 {
		if err != nil {
			return nil, utils.Errorf(`cwe database[%v] failed: %s`, cveDatabase, err)
		}
	}
	return gormCVEDatabase, nil
}
