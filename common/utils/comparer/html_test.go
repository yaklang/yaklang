package comparer

import (
	"github.com/davecgh/go-spew/spew"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestCompareHtml(t *testing.T) {
	rsp1, err := http.Get("https://www.baidu.com/123123123")
	if err != nil {
		panic(err)
	}

	rsp2, err := http.Get("http://www.baidu.com/12aaa")
	if err != nil {
		panic(err)
	}

	body1, _ := ioutil.ReadAll(rsp1.Body)
	body2, _ := ioutil.ReadAll(rsp2.Body)

	spew.Dump(CompareHtml(body1, body2))
}
