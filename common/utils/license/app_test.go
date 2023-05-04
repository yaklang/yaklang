package license

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"testing"
	"time"
)

func TestNewMachine(t *testing.T) {
	test := assert.New(t)
	pri1, pub1, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		test.FailNow(err.Error())
	}

	pri2, pub2, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		test.FailNow(err.Error())
	}

	m1, m2 := NewMachine(pub1, pri2), NewMachine(pub2, pri1)

	req, err := m1.GenerateRequest()
	if err != nil {
		test.FailNow(err.Error())
	}

	spew.Dump("Request: ", req)

	licenseRaw, err := m2.SignLicense(req, "Test", 10*time.Second, nil)
	if err != nil {
		test.FailNow(err.Error())
	}

	spew.Dump("Response: ", licenseRaw)

	rsp, err := m1.VerifyLicense(licenseRaw)
	if err != nil {
		test.FailNow(err.Error())
	}

	spew.Dump(rsp)
}
