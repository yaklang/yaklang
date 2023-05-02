package codec

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
	"yaklang/common/log"
)

func TestRC4EncAndDec(t *testing.T) {
	bytes, err := RC4Encrypt([]byte("test"), []byte("a123456"))
	if err != nil {
		panic(err)
	}
	spew.Dump(EncodeBase64(bytes))

	origin, err := RC4Decrypt([]byte(`test`), bytes)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)
	assert.Equal(t, []byte("a123456"), origin)
}

func TestRC4Dec(t *testing.T) {

	cipher, err := DecodeBase64("473xeG4wtlxIPuzM0Zi46bl2MAt6rc4g/puO1N6uKMor9D6bGuJ0E+OGMIpQcIHoJyPV/W7zr5MNMEDUDkCslc2fgzkOTgFGjjeSbzmMBZpdY7MhAtH6tQRCb3TzGMHZY1dnGPCovFrL5NT9Wse7ILNwkN1sEk51koKOXcIcAlmFhd3bSL8R5+5irXmbEH1SdiyPdQr5L9DBGMBGCYUwY6qsOcn9RE0b1p+/LcVxji+/PKrmCG52YnFTJfezjzi681DqjQUq9RrN3w9IQV8tkfVzzV7UrN4TfDSHZDHeofjBMNs/Cn0e8lnga6Kw5nOuO2BKkByRyWvjrzT1sMGavQuxPMcNkNqklOhc+JqRn9gDGLmWp7hHwcBj8xoes1SSyY/t/RPrmK/wOqmfONvjVpy73UIQ1g==")

	origin, err := RC4Decrypt([]byte(`abcsdfadfjiweur`), cipher)
	if err != nil {
		panic(err)
	}
	spew.Dump(origin)
	log.Println(string(origin))
	assert.Contains(t, string(origin), `phpinfo()`)
}
