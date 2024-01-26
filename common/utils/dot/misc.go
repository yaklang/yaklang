package dot

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func DotGraphToAsciiArt(t string) (string, error) {
	if utils.InGithubActions() {
		return "", utils.Errorf("not exec in github actions")
	}
	if !utils.InTestcase() {
		return "", utils.Error("only in testcase local")
	}
	// `https://dot-to-ascii.ggerganov.com/`
	rsp, _, err := poc.HTTP(`GET /dot-to-ascii.php?boxart=0&src= HTTP/1.1
Host: dot-to-ascii.ggerganov.com
Connection: keep-alive
sec-ch-ua: "Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"
sec-ch-ua-mobile: ?0
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36
sec-ch-ua-platform: "macOS"
Accept: */*
Sec-Fetch-Site: same-origin
Sec-Fetch-Mode: cors
Sec-Fetch-Dest: empty
Referer: https://dot-to-ascii.ggerganov.com/
Accept-Encoding: gzip, deflate, br
Accept-Language: zh-CN,zh;q=0.9
`, poc.WithReplaceHttpPacketQueryParam("src", t), poc.WithForceHTTPS(true), poc.WithTimeout(15), poc.WithProxy("127.0.0.1:7890"))
	if err != nil {
		return "", err
	}
	_, body := lowhttp.SplitHTTPPacketFast(rsp)
	return codec.UnescapeHtmlString(string(body)), nil
}

func ShowDotGraphToAsciiArt(t string) (string, error) {
	graph, err := DotGraphToAsciiArt(t)
	if err != nil {
		log.Warnf("dot graph to ascii art failed: %v", err)
	} else {
		fmt.Println(graph)
	}
	return graph, err
}
