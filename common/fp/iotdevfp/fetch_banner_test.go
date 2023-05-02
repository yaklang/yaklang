package iotdevfp

import (
	"context"
	"testing"
	"yaklang/common/utils/lowhttp"
)

func TestFetchBannerFromHostPort(t *testing.T) {
	banner := lowhttp.FetchBannerFromHostPort(context.Background(), "qq.com", 443, 2048, false, false, false)
	println(string(banner))
}
