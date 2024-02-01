package fp

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/dlclark/regexp2"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"io/ioutil"
	"regexp"
	"testing"
)

func TestRDPRegexp2MatchFailed(t *testing.T) {
	rcom := regexp.MustCompilePOSIX(`.*\xd0.*`)
	s := rcom.FindString(utils2.AsciiBytesToRegexpMatchedString([]byte("\x03\x00\x00\x13\x0e\xd0\x00\x00\x124\x00\x02\x00\x08\x00\x02\x00\x00\x00")))
	if s == "" {
		t.Logf("regexp failed")
		t.Fail()
	}
	//
	log.Infof("%+q\n", "Ð")
	log.Infof("%+q\n", "\xd0")
	log.Infof("%+q\n", []byte("\x00\xd0")[1])
	log.Infof("%#v\n", []byte("\x00\xd0"))
	log.Infof("%+q\n", 0xd0)
	log.Infof("%+q\n", rune(0xd0))

	if utils2.StringToAsciiBytes("Ð")[0] != 0xd0 {
		log.Info(spew.Sprintln([]byte("Ð")))
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
