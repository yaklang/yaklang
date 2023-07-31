// Package httpbrute
// @Author bcy2007  2023/6/20 15:08
package httpbrute

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func HttpBruteForce(urlStr string, opts ...BruteConfigOpt) (chan Result, error) {
	ch := make(chan Result)
	opts = append(opts, WithResultChannel(ch))
	bruteForceModule, err := NewBruteForce(urlStr, opts...)
	if err != nil {
		close(ch)
		return ch, utils.Errorf(`[bruteforce] Create New BruteForce Error: %v`, err.Error())
	}
	go func() {
		defer close(ch)
		err := bruteForceModule.Start()
		if err != nil {
			log.Errorf("[simulator] http brute force running error: %s", err.Error())
		}
	}()
	return ch, nil
}
