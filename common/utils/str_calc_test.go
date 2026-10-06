package utils

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestSimilarityHash(t *testing.T) {
	test := assert.New(t)
	token := RandStringBytes(20000)
	tokenShortFe := token[:17000]
	tokenShortEn := token[:15000]
	_ = tokenShortEn
	_ = tokenShortFe
	docs := [][]byte{
		//[]byte(token),
		[]byte(tokenShortFe),
		[]byte(tokenShortEn),
		//[]byte(token[233:]),
		//[]byte(token[2456:12657]),
		//[]byte(tokenShortEn),
	}

	stability, err := CalcSSDeepStability(docs...)
	if err != nil {
		test.FailNow(err.Error())
	}
	spew.Dump(stability)

	stability, err = CalcSimHashStability(docs...)
	if err != nil {
		test.FailNow(err.Error())
	}
	spew.Dump(stability)
}

func TestSimilarStr(t *testing.T) {
	rand := RandStringBytes(20)
	spew.Dump(GetSameSubStrings(
		"asdf123123123123123123jklasdfajsdf123123as"+rand+";fnlasdfnasdfaaaaaaa123123123123123123123123123123123123123123123123123123123123123123123123",
		"asdf12312312312312312"+rand+"3123123123123123123123123123123123123jklasdfajsdf123123123123123123123123asasdfjklasd;jla;fnlasdfnasdfaaaaaaa",
		//"12312312312312312312312312312312312312312312"+rand+"3123123123123123123123123123123123123123123123123123123123123123",
	))
}

func TestSimilarStrBIG(t *testing.T) {
	// Keep the large-input correctness guard; repeated throughput measurement
	// belongs in the benchmark rather than every ordinary test run.
	raw := largeSimilarityInput()
	assert.Equal(t, 1.0, CalcSimilarity(raw, raw))
}

func BenchmarkCalcSimilarityLarge(b *testing.B) {
	raw := largeSimilarityInput()
	b.ReportAllocs()
	b.SetBytes(int64(2 * len(raw)))
	b.ResetTimer()
	var score float64
	for i := 0; i < b.N; i++ {
		score = CalcSimilarity(raw, raw)
	}
	if score != 1 {
		b.Fatalf("identical inputs have similarity %v", score)
	}
}

func largeSimilarityInput() []byte {
	rng := rand.New(rand.NewSource(1))
	raw := make([]byte, 900000)
	for i := range raw {
		raw[i] = byte('a' + rng.Intn(26))
	}
	return raw
}

func TestGetSameSubStringsCompatibility(t *testing.T) {
	for _, tc := range []struct {
		in   []string
		want []string
	}{
		{nil, nil}, {[]string{"only"}, nil}, {[]string{"", ""}, nil},
		{[]string{"same", "same"}, nil}, {[]string{"ab", "cd"}, []string{}},
		{[]string{"你好abc", "你好xyz"}, []string{"你好"}},
		{[]string{"abcabc", "abcXabc"}, []string{"abc"}},
		{[]string{"aXXb", "aYYb", "aZZb"}, []string{"a", "b"}},
		{[]string{"aXXb", "aYYb", "aZZb", "aWWb"}, []string{"a", "b"}},
		{[]string{"aXXb", "aYYb", "aZZb", "none"}, []string{}},
	} {
		got := GetSameSubStrings(tc.in...)
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %#v want %#v", tc.in, got, tc.want)
		}
	}
}
