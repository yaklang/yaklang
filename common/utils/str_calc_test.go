package utils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
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
	println(time.Now().String())
	for range make([]interface{}, 100) {
		rand := RandStringBytes(900000)
		r := CalcSimilarity([]byte(rand), []byte(rand))
		_ = r
	}
	println(time.Now().String())
}
