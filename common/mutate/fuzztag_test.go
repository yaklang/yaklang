package mutate

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestLower(t *testing.T) {
	var a = fuzzLowerNUpper("zhangsan")
	spew.Dump(a)
}

func TestMutateDoc(t *testing.T) {

	var GetFuzztagMarkdownDoc = func() string {
		/*
			表格内
			|标签名|标签别名|标签描述|
			|:--------|:-------|:------|

		*/
		var buf bytes.Buffer
		buf.Write([]byte(`

## fuzztag 可用标签一览

|标签名|标签别名|标签描述|
|:-------|:-------|:-------|
`))
		escapeVertical := func(s string) string {
			return strings.ReplaceAll(s, `|`, `&#124;`)
		}
		sort.SliceStable(existedFuzztag, func(i, j int) bool {
			return existedFuzztag[i].TagName < existedFuzztag[j].TagName
		})
		for _, t := range existedFuzztag {
			aliasName := escapeVertical(strings.Join(t.Alias, ", "))
			if aliasName != "" {
				aliasName = "`" + aliasName + "`"
			} else {
				aliasName = "  "
			}
			buf.WriteString(
				fmt.Sprintf("|`%v`|%v|%v|",
					escapeVertical(t.TagName),
					aliasName,
					escapeVertical(t.Description),
				),
			)
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
		buf.WriteByte('\n')
		return buf.String()
	}

	println(GetFuzztagMarkdownDoc())
}

func TestMutateQuick(t *testing.T) {
	var results []string

	results = MutateQuick(`{{int(1-29)}},-asdfasdfasd{{randstr({{int(1-20)}},100,2)}}`)
	if len(results) != 29*20*2 {
		panic(len(results))
	}

	results = MutateQuick(`{{repeatstr(abc,|{{int(1-10)}})}}`)
	if len(results) != 10 {
		panic(len(results))
	}

	results = MutateQuick(`select {{randomupper({{repeatstr(asdfasdfasdf|{{int(1-5)}})}} 1hjkzxdnkj)}}`)
	/*
		([]string) (len=5 cap=5) {
		 (string) (len=30) "select aSDfasDfasDf 1hJkzXdNKJ",
		 (string) (len=42) "select asdFaSDfasdfaSdfaSDFaSDF 1HJKzxdnkj",
		 (string) (len=54) "select aSdfaSdFasdFasDfaSdfasDfasdfasDfasDF 1hJKzXdnkJ",
		 (string) (len=66) "select aSDFasDFasDfaSDfasDfasdfaSdFasDFasDFaSDFasDFaSDF 1HjKzxdnkj",
		 (string) (len=78) "select asDFasDfaSDfasdFaSDFasdfasDfasDfasDFasDfaSDfaSdfaSDFaSdFaSDf 1HjkzxdNkJ"
		}
	*/
	spew.Dump(results)
	if len(results) != 5 {
		panic(len(results))
	}

	results = MutateQuick(`select {{randomupper({{repeatstr(asdfasdfasdf|{{int(1-5)}})}} 1hjkzxdnkj)}}`)
	/*
		([]string) (len=5 cap=5) {
		 (string) (len=30) "select aSDfasDfasDf 1hJkzXdNKJ",
		 (string) (len=42) "select asdFaSDfasdfaSdfaSDFaSDF 1HJKzxdnkj",
		 (string) (len=54) "select aSdfaSdFasdFasDfaSdfasDfasdfasDfasDF 1hJKzXdnkJ",
		 (string) (len=66) "select aSDFasDFasDfaSDfasDfasdfaSdFasDFasDFaSDFasDFaSDF 1HjKzxdnkj",
		 (string) (len=78) "select asDFasDfaSDfasdFaSDFasdfasDfasDfasDFasDfaSDfaSdfaSDFaSdFaSDf 1HjkzxdNkJ"
		}
	*/
	if len(results) != 5 {
		panic(len(results))
	}

	results = MutateQuick(`select {{ri(0,99999,10|20)}}`)
	/*
		([]string) (len=5 cap=5) {
		 (string) (len=30) "select aSDfasDfasDf 1hJkzXdNKJ",
		 (string) (len=42) "select asdFaSDfasdfaSdfaSDFaSDF 1HJKzxdnkj",
		 (string) (len=54) "select aSdfaSdFasdFasDfaSdfasDfasdfasDfasDF 1hJKzXdnkJ",
		 (string) (len=66) "select aSDFasDFasDfaSDfasDfasdfaSdFasDFasDFaSDFasDFaSDF 1HjKzxdnkj",
		 (string) (len=78) "select asDFasDfaSDfasdFaSDFasdfasDfasDfasDFasDfaSDfaSdfaSDFaSdFaSDf 1HjkzxdNkJ"
		}
	*/
	if len(results) != 10 {
		panic(len(results))
	}

	results = MutateQuick(`x {{int(123123,1)}}{{x(aaa)}}`)
	/*
		([]string) (len=5 cap=5) {
		 (string) (len=30) "select aSDfasDfasDf 1hJkzXdNKJ",
		 (string) (len=42) "select asdFaSDfasdfaSdfaSDFaSDF 1HJKzxdnkj",
		 (string) (len=54) "select aSdfaSdFasdFasDfaSdfasDfasdfasDfasDF 1hJKzXdnkJ",
		 (string) (len=66) "select aSDFasDFasDfaSDfasDfasdfaSdFasDFasDFaSDFasDFaSDF 1HjKzxdnkj",
		 (string) (len=78) "select asDFasDfaSDfasdFaSDFasdfasDfasDfasDFasDfaSDfaSdfaSDFaSdFaSDf 1HjkzxdNkJ"
		}
	*/
	spew.Dump(results)
	if len(results) != 2 {
		panic(len(results))
	}

	results = MutateQuick(`{{int(1-20||2)}}`)
	spew.Dump(results)
	if len(results) != 10 {
		panic(len(results))
	}
}
func TestAlias(t *testing.T) {
	results := MutateQuick(`{{rs(2)}}`)
	spew.Dump(results)
}
func TestYsoFuzzTag(t *testing.T) {
	result := MutateQuick(`{{yso:exec(whoami)}}`)
	println(len(result))
	spew.Dump(result)
}

func TestRegenTag(t *testing.T) {
	result := MutateQuick(`{{regen(aa*)}}`)
	println(len(result))
	spew.Dump(result)
}

func TestIntWithAutoZeroPadding(t *testing.T) {
	t.Run("000-999", func(t *testing.T) {
		result := MutateQuick(`{{int(000-999)}}`)
		for i := 0; i < 100; i++ {
			r := result[i]
			count := 2
			if i >= 9 {
				count = 1
			} else if i >= 99 {
				count = 0
			}
			if !strings.HasPrefix(r, strings.Repeat("0", count)) {
				t.Fatalf("%s padding zero in left error: want %d zero", r, count)
			}
		}
	})

	t.Run("011-999", func(t *testing.T) {
		result := MutateQuick(`{{int(011-999)}}`)
		for i := 0; i < 100; i++ {
			r := result[i]
			count := 1
			if i >= 9 {
				count = 0
			}
			if !strings.HasPrefix(r, strings.Repeat("0", count)) {
				t.Fatalf("%s padding zero in left error: want %d zero", r, count)
			}
		}
	})

	t.Run("01-999", func(t *testing.T) {
		result := MutateQuick(`{{int(01-999)}}`)
		for i := 0; i < 100; i++ {
			r := result[i]
			count := 1
			if i >= 9 {
				count = 0
			}
			if !strings.HasPrefix(r, strings.Repeat("0", count)) {
				t.Fatalf("%s padding zero in left error: want %d zero", r, count)
			}
		}
	})
}

func TestRepeatTag(t *testing.T) {
	result := MutateQuick(`{{repeat(!|4)}}`)
	println(len(result))
	spew.Dump(result)
}
