// Package httpbrute
// @Author bcy2007  2023/6/20 15:08
package httpbrute

import "github.com/yaklang/yaklang/common/log"

func HttpBruteForce(urlStr string, opts ...BruteConfigOpt) (chan Result, error) {
	ch := make(chan Result)
	opts = append(opts, WithResultChannel(ch))
	bruteForceModule := NewBruteForce(urlStr, opts...)
	go func() {
		defer close(ch)
		err := bruteForceModule.Start()
		if err != nil {
			log.Errorf("[simulator] http brute force running error: %s", err.Error())
		}
	}()
	return ch, nil
}
