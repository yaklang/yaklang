package xlic

import (
	"sync"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/license"

	"github.com/jinzhu/gorm"
)

var EncPub = `
-----BEGIN RSA PUBLIC KEY-----
MIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA03IMmS7ox5mmBBTnuqeq
VQe3K26Qh9GMzqxO35G5SEzfChHcodkJq+7/tHOcWFt1Qm8OUz6VbzqCM9E0VFMe
yH4lB0AZM3js2uxWmHJ7UI8EyMWPNUwHVxePehILiXJRHlLohtoU/JsODx6DDwTR
dxNu7yEC7p5yH/RmHzJP9N5NRIz+l00zAGkzkjb1gkhNQrR3CHH/7hfTDsexBJgv
OyBxKS5q5aiZ9cQ+oM8vD/meTWOTel82D96ipvxjyyffL6BK/CSPRfOSJTsOvFB2
8nQgRAEmU9+LooQ/rzbB+hELnj1rh4DkZ/eAGsls7ZgWcKyZHea6gkY2A65BUxPI
F1oEc2yIKCfU0jZbL+Fwb3Pg7ORQIZOp50zuqc5fiFP9TYiJY/bOVe0aMi6+OdlW
8qNVpfJhD9pnxjR7fd8ekVz7Fx4M+xml3k/e+yZ7rwiMW/Md9tuSWmUT7/eqOvnu
js0brbNhgoBUh0uCm5DVJINqE/8P+LWw2K2zmM14g5ocxv8j2OaoxPOcEz7iiPx6
01IhKCJ4qEL4MGpUModenGdB+6q0Rtl2mXpZdlYXK9Km0ANlSljrPVBGk5TEapdy
/6bp6oQbzyyuflAjmjnjTMFyL3mHuRomVrBU2mD11e7uuaoS0tbnL+UnxDP4GcIB
24wbKmzYzx3La8zEnAjTa8sCAwEAAQ==
-----END RSA PUBLIC KEY-----`

var DecPri = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----`

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
