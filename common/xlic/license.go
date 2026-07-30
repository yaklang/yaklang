package xlic

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/license"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"sync"

	"github.com/yaklang/gorm"
)

// Only embed the PUBLIC key. The private key (pri.gzip) is NOT embedded — it
// must never ship in the client binary. The signer (xlic CLI / signing server)
// holds the private key out-of-band. If pri.gzip is absent the verifier still
// works (it only needs the public key); if pub.gzip is absent we fall back to
// a generated dev keypair (local dev only).
//
//go:embed certs/pub.gzip
var pubGzip []byte

var (
	initOnce sync.Once
	Machine  *license.Machine
)

func initMachine() {
	initOnce.Do(func() {
		var encBytes []byte

		if len(pubGzip) > 0 {
			if raw, _ := utils.GzipDeCompress(pubGzip); len(raw) > 0 {
				encBytes = raw
			}
		}

		if len(encBytes) <= 0 {
			// dev fallback: no embedded public key — generate a throwaway keypair
			encBytes, _, _ = tlsutils.GeneratePrivateAndPublicKeyPEM()
		}

		// Verifier-only: the shipped binary only needs the public key to verify
		// licenses. The private key is held by the off-box signer, never here.
		Machine = license.NewVerifier(encBytes)
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
