package protocol_impl

import (
	"bytes"
	"encoding/asn1"
	"testing"
)

type Sub struct {
	S string `asn1:"explicit,tag:0"`
}
type TestBerStruct struct {
	Integer int    `asn1:"explicit,tag:0"`
	B       bool   `asn1:"explicit,tag:1"`
	Str     string `asn1:"explicit,tag:2"`
	Self    Sub    `asn1:"explicit,tag:3"`
}

func TestBer(t *testing.T) {
	res, err := asn1.Marshal(TestBerStruct{
		Integer: 1,
		B:       true,
		Str:     "s",
		Self:    Sub{S: "aa"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ParseBER(bytes.NewReader(res))
}
