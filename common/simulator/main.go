// Package simulator
// @Author bcy2007  2023/8/22 13:35
package simulator

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func HttpBruteForce(targetUrl string, opts ...BruteConfigOpt) (chan Result, error) {
	ch := make(chan Result)
	opts = append(opts, WithResultChannel(ch))
	bruteForce, err := NewHttpBruteForceCore(targetUrl, opts...)
	if err != nil {
		close(ch)
		return nil, utils.Error(err)
	}
	go func() {
		defer close(ch)
		err := bruteForce.Start()
		if err != nil {
			log.Error(err)
		}
	}()
	return ch, nil
}
