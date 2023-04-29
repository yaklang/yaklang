package rpa

import (
	"yaklang/common/log"
	"yaklang/common/rpa/core"
)

func Start(url string, opt ...core.ConfigOpt) (chan core.RequestIf, error) {
	ch := make(chan core.RequestIf)
	opt = append(opt, core.WithOnRequest(func(req *core.Req) {
		ch <- req
	}))
	manager, err := core.NewManager(
		url,
		ch,
		opt...,
	)
	if err != nil {
		// panic(err)
		return nil, err
	}

	go func() {
		defer close(ch)
		err = manager.Run()
		if err != nil {
			// panic(err)
			log.Infof("manager run error:%s", err)
			return
		}
	}()

	return ch, nil
}
