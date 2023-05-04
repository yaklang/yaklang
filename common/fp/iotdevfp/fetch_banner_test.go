package iotdevfp

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"testing"
)

func TestFetchBannerFromHostPort(t *testing.T) {
	banner := lowhttp.FetchBannerFromHostPort(context.Background(), "qq.com", 443, 2048, false, false, false)
	println(string(banner))
}
