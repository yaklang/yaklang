package license

import (
	"encoding/base64"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"strings"
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

func TestSignedLicenseV2RoundTripAndTamper(t *testing.T) {
	pri, pub, err := tlsutils.GeneratePrivateAndPublicKeyPEM()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	requester := NewMachine(pub, nil)
	signer := NewMachine(nil, pri)
	req, err := requester.GenerateRequest()
	if err != nil {
		t.Fatalf("generate request: %v", err)
	}
	licenseRaw, err := signer.SignLicenseV2(req, "Test", 24*time.Hour, map[string]string{
		"entitlements": `{"products":["hids"],"version":1}`,
	})
	if err != nil {
		t.Fatalf("sign v2 license: %v", err)
	}
	rsp, err := requester.VerifyLicenseV2(licenseRaw)
	if err != nil {
		t.Fatalf("verify v2 license: %v", err)
	}
	if rsp.Org != "Test" {
		t.Fatalf("unexpected org: %q", rsp.Org)
	}

	parts := strings.Split(licenseRaw, ".")
	if len(parts) != 3 || parts[0] != SignedLicenseV2Prefix {
		t.Fatalf("unexpected v2 format: %q", licenseRaw)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if err := json.Unmarshal(payload, &rsp); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	rsp.NotAfterTimestamp = time.Now().Add(10 * 365 * 24 * time.Hour).Unix()
	payload, err = json.Marshal(rsp)
	if err != nil {
		t.Fatalf("marshal tampered payload: %v", err)
	}
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	if _, err := requester.VerifyLicenseV2(strings.Join(parts, ".")); err == nil {
		t.Fatal("tampered v2 payload must fail verification")
	}
}
