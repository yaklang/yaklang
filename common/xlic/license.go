package xlic

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/license"
	"sync"

	"github.com/jinzhu/gorm"
)

var EncPub = `
`

var DecPri = ``

var (
	initOnce sync.Once
	Machine  *license.Machine
)

func initMachine() {
	initOnce.Do(func() {
		Machine = license.NewMachine([]byte(EncPub), []byte(DecPri))
	})
}

func init() {
	initMachine()
}

type License struct {
	License string `gorm:"unique"`
}

func VerifyAndSaveLicense(db *gorm.DB, license string) error {
	initMachine()

	_, err := Machine.VerifyLicense(license)
	if err != nil {
		return err
	}

	var lic = &License{
		License: license,
	}
	if db := db.Model(&License{}).Where("true").Unscoped().Delete(&License{}); db.Error != nil {
		log.Error(db.Error)
		return utils.Errorf("remove old legacy failed: %s", db.Error)
	}

	if db := db.Save(lic); db.Error != nil {
		return utils.Errorf("save lic error: %s", db.Error)
	}

	return nil
}

func LoadAndVerifyLicense(db *gorm.DB) (*license.Response, error) {
	initMachine()

	var lic License
	if db := db.Model(&License{}).First(&lic); db.Error != nil {
		return nil, utils.Errorf("fetch license from db failed: %s", db.Error)
	}
	rsp, err := Machine.VerifyLicense(lic.License)
	if err != nil {
		return nil, err
	}
	return rsp, nil
}

func GetLicenseRequest() (string, error) {
	initMachine()

	return Machine.GenerateRequest()
}

func RemoveLicense(db *gorm.DB) {
	if db := db.Model(&License{}).Delete(&License{}); db.Error != nil {
		log.Error("remove license error: %s", db.Error)
		return
	}
}
