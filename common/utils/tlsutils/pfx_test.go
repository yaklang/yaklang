package tlsutils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestP12OrPFX(t *testing.T) {
	ca, key, err := GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, sKey, err := SignServerCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, err := BuildP12(cert, sKey, "123456", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err := LoadP12ToPEM(p12Bytes, "123456")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)

	p12Bytes, err = BuildP12(cert, sKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err = LoadP12ToPEM(p12Bytes, "")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)
}
