package fp

import (
	"bytes"
	"fmt"
	"github.com/dlclark/regexp2"
	"github.com/k0kubun/pp"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io/ioutil"
	"regexp"
	"testing"
	"yaklang.io/yaklang/common/log"
	utils2 "yaklang.io/yaklang/common/utils"
)

func TestRDPRegexp2MatchFailed(t *testing.T) {
	rcom := regexp.MustCompilePOSIX(`.*\xd0.*`)
	s := rcom.FindString(utils2.AsciiBytesToRegexpMatchedString([]byte("\x03\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00")))
	if s == "" {
		t.Logf("regexp failed")
		t.Fail()
	}
	//
	//pp.Println("TARGET", []byte("Ð"), " | ", string([]byte{0xd0}))
	//pp.Println("DATA SOURCE", []byte("\xd0"))
	//
	//pp.Print("\xd0 encode to ")
	//pp.Println(UTF8Encode([]byte("\xd0")))
	//
	////pp.Print("     decode to ")
	////pp.Println(unicode.UTF8.NewDecoder().Bytes([]byte("\xd0")))
	//
	//pp.Print("Ð encode to")
	//pp.Println(UTF8Encode([]byte("Ð")))

	log.Infof("%+q\n", "Ð")
	log.Infof("%+q\n", "\xd0")
	log.Infof("%+q\n", []byte("\x00\xd0")[1])
	log.Infof("%#v\n", []byte("\x00\xd0"))
	log.Infof("%+q\n", 0xd0)
	log.Infof("%+q\n", rune(0xd0))

	//pp.Print("  decode to ")
	//pp.Println(unicode.UTF8.NewDecoder().Bytes([]byte("Ð")))

	if utils2.StringToAsciiBytes("Ð")[0] != 0xd0 {
		log.Info(pp.Sprintln([]byte("Ð")))
		t.FailNow()
	}

	var res string
	for _, b := range []byte("\x03\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00") {
		res += fmt.Sprintf("%s", string(b))
	}
	//pp.Println(StringToAsciiBytes("Ð"))

	reader := transform.NewReader(bytes.NewReader([]byte("Ð")), unicode.UTF8.NewDecoder())
	_, _ = ioutil.ReadAll(reader)

	com := regexp2.MustCompile(`\xc0`, regexp2.IgnorePatternWhitespace)
	a, _ := com.FindRunesMatch(utils2.AsciiBytesToRegexpMatchedRunes([]byte("\x03\xc0\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00")))
	if a == nil {
		t.Logf("\\xc0 find pattern failed")
		t.FailNow()
	}

	com = regexp2.MustCompile(`\x03\x00\x00.{2}.\x00`, 0)
	a, _ = com.FindStringMatch("\x03\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00")
	if a == nil {
		t.Logf("find pattern failed")
		t.FailNow()
	}
}
