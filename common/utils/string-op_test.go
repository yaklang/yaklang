package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"testing"
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

func TestPrettifyListFromStringSplit(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g"}, PrettifyListFromStringSplitEx("a,b,c|d|e,f|g", ",", "|"))
	assert.Equal(t, []string{"a", "b", "c", "d", "e", "f", "g"}, PrettifyListFromStringSplitEx("a,b,c d|e,f|g", ",", "|", " "))
	assert.Equal(t, []string{"a", "b", "c"}, PrettifyListFromStringSplitEx("abc", ""))
	assert.Equal(t, []string{"a", "b", "c"}, PrettifyListFromStringSplitEx("a b c", " "))
}

func TestPrettifyJoin(t *testing.T) {
	assert.Equal(t, "a,b,c,d,e,f,g", PrettifyJoin(",", "a", "b", "c", "d", "e", "f", "g"))
	assert.Equal(t, "a,b,c,f,g", PrettifyJoin(",", "a", "b", "c", "", "", "f", "g"))
	assert.Equal(t, "a,b,c,f", PrettifyJoin(",", "a", "b", "c", "", "", "f", ""))
	assert.Equal(t, "b,c,f,g", PrettifyJoin(",", "", "b", "c", "", "", "f", "g"))
	assert.Equal(t, "c,f,g", PrettifyJoin(",", "", "", "c", "", "", "f", "g"))
	assert.Equal(t, "c,f", PrettifyJoin(",", "", "", "c", "", "", "f", ""))
	assert.Equal(t, "c,f,c,f,c,f,c,f,c,f,c,f", PrettifyJoin(",", "", "", "c", "", "", "f", "", "c", "f", "c", "f", "c", "f", "c", "f", "c", "f"))
	assert.Equal(t, "c,f", PrettifyShrinkJoin(",", "", "", "c", "", "", "f", "", "c", "f", "c", "f", "c", "f", "c", "f", "c", "f"))
}
