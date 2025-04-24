package mutate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
)

// func TestLower(t *testing.T) {
// 	a := fuzzLowerNUpper("zhangsan")
// 	spew.Dump(a)
// }

// func TestMutateDoc(t *testing.T) {
// 	GetFuzztagMarkdownDoc := func() string {
// 		/*
// 			表格内
// 			|标签名|标签别名|标签描述|
// 			|:--------|:-------|:------|

// 		*/
// 		var buf bytes.Buffer
// 		buf.Write([]byte(`

// ## fuzztag 可用标签一览

// |标签名|标签别名|标签描述|
// |:-------|:-------|:-------|
// `))
// 		escapeVertical := func(s string) string {
// 			return strings.ReplaceAll(s, `|`, `&#124;`)
// 		}
// 		sort.SliceStable(existedFuzztag, func(i, j int) bool {
// 			return existedFuzztag[i].TagName < existedFuzztag[j].TagName
// 		})
// 		for _, t := range existedFuzztag {
// 			aliasName := escapeVertical(strings.Join(t.Alias, ", "))
// 			if aliasName != "" {
// 				aliasName = "`" + aliasName + "`"
// 			} else {
// 				aliasName = "  "
// 			}
// 			buf.WriteString(
// 				fmt.Sprintf("|`%v`|%v|%v|",
// 					escapeVertical(t.TagName),
// 					aliasName,
// 					escapeVertical(t.Description),
// 				),
// 			)
// 			buf.WriteByte('\n')
// 		}
// 		buf.WriteByte('\n')
// 		buf.WriteByte('\n')
// 		return buf.String()
// 	}

// 	println(GetFuzztagMarkdownDoc())
// }

func TestMutateQuick(t *testing.T) {
	var results []string

	// results = MutateQuick(`{{int(1-29)}},-asdfasdfasd{{randstr({{int(1-20)}},100,2)}}`)
	// if len(results) != 29*20*2 {
	// 	panic(len(results))
	// }

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

// func TestAlias(t *testing.T) {
// 	results := MutateQuick(`{{rs(2)}}`)
// 	spew.Dump(results)
// }

// func TestYsoFuzzTag(t *testing.T) {
// 	result := MutateQuick(`{{yso:exec(whoami)}}`)
// 	println(len(result))
// 	spew.Dump(result)
// }

// func TestRegenTag(t *testing.T) {
// 	result := MutateQuick(`{{regen(aa*)}}`)
// 	println(len(result))
// 	spew.Dump(result)
// }

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

// func TestRepeatTag(t *testing.T) {
// 	result := MutateQuick(`{{repeat(!|4)}}`)
// 	println(len(result))
// 	spew.Dump(result)
// }

func TestFuzzTagExec(t *testing.T) {
	expect := []string{
		"a", "a,1,1",
		"a", "a,[__YakHotPatchErr@strconv.Atoi: parsing \"a\": invalid syntax]",
		"a", "a,2,2",
	}
	i := 0
	_, err := FuzzTagExec("{{a({{int({{array(1|a|2)}})}})}}", Fuzz_WithResultHandler(func(s string, payloads []string) bool {
		if s+strings.Join(payloads, ",") != expect[i]+expect[i+1] {
			t.Fatal("test verbose info failed")
		}
		i += 2
		return true
	}), Fuzz_WithExtraFuzzTagHandler("int", func(s string) []string {
		_, err := strconv.Atoi(s)
		if err != nil {
			panic(err)
		}
		return []string{s}
	}), Fuzz_WithExtraFuzzTagHandler("a", func(s string) []string {
		return []string{"a"}
	}))
	// res, err := FuzzTagExec("{{uuid(a)}}")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDynFuzzTag(t *testing.T) {
	expect := []string{
		"aa",
		"ba",
		"ca",
	}
	resi := 0
	i1 := 0
	i2 := 0
	randstrList := []string{"a", "b", "c"}
	_, err := FuzzTagExec("{{randstr1()}}{{randstr2()}}{{repeat(3)}}", Fuzz_WithExtraDynFuzzTagHandler("randstr1", func(s string) []string {
		defer func() {
			i1++
		}()
		return []string{randstrList[i1]}
	}), Fuzz_WithExtraFuzzTagHandler("randstr2", func(s string) []string {
		defer func() {
			i2++
		}()
		return []string{randstrList[i2]}
	}), Fuzz_WithExtraFuzzTagHandler("repeat", func(s string) []string {
		n, err := strconv.Atoi(s)
		if err != nil {
			panic(err)
		}
		res := []string{}
		for range make([]int, n) {
			res = append(res, "")
		}
		return res
	}), Fuzz_WithResultHandler(func(s string, i []string) bool {
		if s != expect[resi] {
			t.Fatal("test verbose info failed")
		}
		resi++
		return true
	}))
	// res, err := FuzzTagExec("{{uuid(a)}}")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFuzzTagBug(t *testing.T) {
	times := 0
	_, err := FuzzTagExec("{{ri(0,9,3)}}{{ri(0,9,3)}}", Fuzz_WithResultHandler(func(s string, payloads []string) bool {
		times++
		return true
	}))
	// res, err := FuzzTagExec("{{uuid(a)}}")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 9, times)

	times = 0
	_, err = FuzzTagExec("{{ri(0,9,3)}}{{ri(0,9,3)}}{{repeat(9)}}", Fuzz_WithResultHandler(func(s string, payloads []string) bool {
		times++
		return true
	}))
	// res, err := FuzzTagExec("{{uuid(a)}}")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 81, times)
}

func TestDateFuzzTagLocation(t *testing.T) {
	require.Contains(t, MutateQuick(`{{date(YYYY-MM-ddZ,UTC)}}`)[0], "+0000")
}

func TestDateRangeFuzzTag(t *testing.T) {
	require.Equal(
		t,
		[]string{
			"20080101", "20080102", "20080103", "20080104", "20080105", "20080106", "20080107", "20080108", "20080109", "20080110", "20080111",
		},
		MutateQuick(`{{date:range(20080101,20080111)}}`),
	)

	require.Equal(
		t,
		[]string{
			"01/01/2008", "01/02/2008", "01/03/2008", "01/04/2008", "01/05/2008", "01/06/2008", "01/07/2008", "01/08/2008", "01/09/2008", "01/10/2008", "01/11/2008",
		},
		MutateQuick(`{{date:range(01/01/2008,01/11/2008)}}`),
	)
}

func TestBigIntFuzztag(t *testing.T) {
	// int
	require.Equal(
		t,
		[]string{`20120301210`, `20120301211`},
		MutateQuick(`{{int(20120301210-20120301211)}}`),
	)

	// randint
	randIntResult := MutateQuick(`{{randint(20120301210,20120301211)}}`)[0]
	if randIntResult != `20120301210` && randIntResult != `20120301211` {
		t.Fatal("randint error")
	}
}

func TestUnicodeTag(t *testing.T) {
	require.Equal(
		t,
		[]string{`\u0031\u0032\u0033\u0034`},
		MutateQuick(`{{unicode:encode(1234)}}`),
	)

	require.Equal(
		t,
		[]string{`1234`},
		MutateQuick(`{{unicode:decode(\u0031\u0032\u0033\u0034)}}`),
	)
}

func TestBigInt(t *testing.T) {
	test := assert.New(t)
	results := MutateQuick(`{{int(100000000000001-100000000000020)}}`)
	test.Equal(20, len(results))
	dosCode := `{{int(0-100000000000020)}}`
	n := 1
	for i := 0; i < 10; i++ {
		dosCode += dosCode
		n *= 2
	}
	start := time.Now()
	_, err := FuzzTagExec(dosCode, Fuzz_WithResultHandler(func(s string, i []string) bool {
		assert.Equal(t, strings.Repeat("0", 1024), s)
		assert.Equal(t, 0, int(time.Now().Sub(start).Seconds()))
		return false
	}))
	if err != nil {
		t.Fatal(err)
	}
}

func TestTagResultLimit(t *testing.T) {
	res, err := FuzzTagExec("{{int(1-10)}}", Fuzz_WithResultLimit(2))
	require.NoError(t, err)
	require.Len(t, res, 2)

	res, err = FuzzTagExec("{{int(1-10)}}")
	require.NoError(t, err)
	require.Len(t, res, 10)
}

func TestFileDir(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "test")
	isExist, err := utils.PathExists(tmpDir)
	assert.NoError(t, err)
	if isExist {
		os.RemoveAll(tmpDir)
	}
	err = os.Mkdir(filepath.Join(os.TempDir(), "test"), os.ModePerm)
	assert.NoError(t, err)
	expect := []string{}
	for i := 0; i < 5; i++ {
		f, err := os.CreateTemp(tmpDir, fmt.Sprintf("test-file%d-*.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		fileContent := fmt.Sprintf("test-file%d", i)
		expect = append(expect, fileContent)
		f.Write([]byte(fileContent))
		f.Close()
	}

	res, err := FuzzTagExec("{{file:dir("+tmpDir+")}}", Fuzz_WithEnableFileTag())
	assert.NoError(t, err)
	assert.Equal(t, expect, res)
}

func TestTimestampFuzzTag(t *testing.T) {
	result := MutateQuick(`{{timestamp(us)}}`)
	require.Len(t, result, 1)
	got, err := strconv.ParseInt(result[0], 10, 64)
	require.NoError(t, err)
	now := time.Now().UnixMicro()
	require.True(t, got >= now-1000 && got <= now+1000)
}

func TestFlowControlTag(t *testing.T) {
	results, err := FuzzTagExec(`{{int(1-2)}}{{int(3-5)}}{{repeat(2)}}`, Fuzz_WithResultHandler(func(s string, i []string) bool {
		return true
	}), Fuzz_SyncTag(true))
	require.NoError(t, err)
	require.Len(t, results, 6)

	results, err = FuzzTagExec(`{{int(1-2)}}{{int(3-5)}}{{repeat(4)}}`, Fuzz_WithResultHandler(func(s string, i []string) bool {
		return true
	}), Fuzz_SyncTag(true))
	require.NoError(t, err)
	require.Len(t, results, 12)
}
