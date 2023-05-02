package crawler

import (
	"testing"
	"yaklang/common/log"
	"yaklang/common/utils"
)

func TestReq_IsLoginForm(t *testing.T) {
	executed := utils.NewBool(false)
	crawler, err := NewCrawler(
		"http://cybertunnel.run:8080",
		WithOnRequest(func(req *Req) {
			println(string(req.Url()))
			executed.Set()
		}),
		WithAutoLogin("admin", "password"),
	)
	if err != nil {
		panic(err)
		return
	}

	err = crawler.Run()
	if err != nil {
		log.Error(err)
		return
	}
	if !executed.IsSet() {
		panic("no exec")
	}
}
