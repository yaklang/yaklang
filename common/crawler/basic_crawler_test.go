package crawler

import "testing"

func TestCrawler_Run(t *testing.T) {
	crawler, err := NewCrawler(
		"http://127.0.0.1:8787/misc/response/javascript-ssa-ir-basic/basic-fetch.html",
		WithOnRequest(func(req *Req) {
			println(req.Url())
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = crawler.Run()
	if err != nil {
		t.Fatal(err)
	}
}
