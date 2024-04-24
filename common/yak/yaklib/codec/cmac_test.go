package codec

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"testing"
)

func TestCmac(t *testing.T) {
	keyByte1, _ := DecodeHex("12345678901234561234567890123456")
	keyByte2, _ := DecodeHex("123456789012345612345678901234561234567890123456")
	res1, _ := DecodeHex("e2c242a76f1f87ff36cde5d069e06011")
	res2, _ := DecodeHex("5bfae9bbdf9e2184")
	cmacAes, err := Cmac("AES", keyByte1, []byte("123"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cmacAes, res1) {
		spew.Dump(cmacAes)
		spew.Dump(res1)
		t.Fatal("cmac AES error")
	}

	cmac3Des, err := Cmac("3DES", keyByte2, []byte("123"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cmac3Des, res2) {
		spew.Dump(cmac3Des)
		spew.Dump(res2)
		t.Fatal("cmac 3DES error")
	}
}
