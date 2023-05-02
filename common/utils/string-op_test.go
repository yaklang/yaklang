package utils

import (
	"fmt"
	"testing"
	"yaklang/common/log"
)

func TestLastLine(t *testing.T) {
	trueCase := map[string]string{
		`aasdfas
asdf
asdf
asdf
aaaa`: "aaaa",
		`aaaa`: "aaaa",
	}

	for k, v := range trueCase {
		if v != string(LastLine([]byte(k))) {
			t.FailNow()
		}
		log.Infof("%s 's last line is %s", k, v)
	}
}

func TestParseStringToVisible(t *testing.T) {
	for i := range make([]int, 256) {
		res := ParseStringToVisible(string([]byte{byte(i)}))
		fmt.Printf("%v (0x%02x)\n", res, i)
	}
}
