package tlsutils

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"
)

func TestGenerateGMSelfSignedCertKey(t *testing.T) {
	c, k, err := GenerateGMSelfSignedCertKey("Yakit GM MITM")
	if err != nil {
		panic(err)
	}

	spew.Dump(c, k)

	scert, sk, err := SignGMServerCrtNKeyWithParams(c, k, "SERVER", time.Now().Add(365*time.Hour*24), false)
	if err != nil {
		panic(err)
	}
	spew.Dump(scert, sk)

	config, err := GetX509GMServerTlsConfigWithAuth(c, scert, sk, false)
	if err != nil {
		panic(err)
	}
	spew.Dump(config)
}
