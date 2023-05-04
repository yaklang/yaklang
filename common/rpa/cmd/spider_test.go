package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa"
	"github.com/yaklang/yaklang/common/rpa/core"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestSpiderRun(t *testing.T) {
	host := "http://testphp.vulnweb.com/"
	depth := 3
	proxy := "127.0.0.1:8099"
	username := ""
	password := ""
	opts := make([]core.ConfigOpt, 2)
	opts = append(opts, core.WithSpiderDepth(depth))
	if proxy != "" {
		if username == "" {
			opts = append(opts, core.WithBrowserProxy(proxy))
		} else {
			opts = append(opts, core.WithBrowserProxy(proxy, username, password))
		}
	}

	rsts, err := rpa.Start(host, opts...)
	if err != nil {
		log.Errorf("spider run error:%s", err)
	}
	// hasPrint filter repeat urls
	hasPrint := filter.NewFilter()
	for rst := range rsts {
		url := rst.Url()
		h := codec.Sha256(url)
		if hasPrint.Exist(h) {
			continue
		} else {
			hasPrint.Insert(h)
			fmt.Println(url)
		}
	}
}
