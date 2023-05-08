package xlic

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/license"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"sync"

	"github.com/jinzhu/gorm"
)

//go:embed certs
var certs embed.FS

var (
	initOnce sync.Once
	Machine  *license.Machine
)

func initMachine() {
	initOnce.Do(func() {
		var (
			encBytes, decBytes []byte
		)

		raw, err := certs.ReadFile("certs/pub.gzip")
		if err != nil {
			log.Debugf("read enc.gzip error: %v", err)
		}
		if len(raw) > 0 {
			if raw, _ := utils.GzipDeCompress(raw); len(raw) > 0 {
				encBytes = raw
			}
		}

		raw, err = certs.ReadFile("certs/pri.gzip")
		if err != nil {
			log.Debugf("read pri.gzip error: %v", err)
		}

		if len(raw) > 0 {
			if raw, _ := utils.GzipDeCompress(raw); len(raw) > 0 {
				decBytes = raw
			}
		}

		if len(encBytes) <= 0 || len(decBytes) <= 0 {
			decBytes, encBytes, _ = tlsutils.GeneratePrivateAndPublicKeyPEM()
		}

		// spew.Dump(codec.Md5(string(encBytes)), codec.Md5(string(decBytes)))
		Machine = license.NewMachine(encBytes, decBytes)
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
		log.Errorf("remove license error: %s", db.Error)
		return
	}
}
