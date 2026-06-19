package comparer

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
)

type Config struct {
	BodyLimit int // default 1024
}

func compareString(s1 string, s2 string) float64 {
	if s1 == "" || s2 == "" {
		return 0
	}
	return utils.CalcSimilarity([]byte(s1), []byte(s2))
}

func compareBytes(s1, s2 []byte) float64 {
	if s1 == nil || s2 == nil {
		return 0
	}
	return utils.CalcSimilarity(s1, s2)
}

func eq(i, b interface{}) float64 {
	if fmt.Sprint(i) == fmt.Sprint(b) {
		return 1
	}
	return 0
}

type score float64

func (s score) Add(i float64, weight float64) score {
	return score(shrink(i*weight + float64(s)))
}

func shrink(f float64) float64 {
	value, err := strconv.ParseFloat(fmt.Sprintf("%.4f", f), 64)
	if err != nil {
		return f
	}
	return value
}

// CompareRaw 比较两个原始 HTTP 响应报文的相似度，返回 0 到 1 之间的相似度分值
// 参数:
//   - rsp1: 第一个原始 HTTP 响应报文
//   - rsp2: 第二个原始 HTTP 响应报文
//
// 返回值:
//   - 相似度分值，1 表示完全相同，0 表示完全不同
//
// Example:
// ```
// // VARS: 比较两个完全相同的响应
// score = judge.CompareRaw("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello", "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello")
// // STDOUT: 打印相似度
// println(score)   // OUT: 1
// // assert: 完全相同的响应相似度为 1
// assert score == 1, "identical responses should score 1"
// ```
func CompareHTTPResponseRaw(rsp1 []byte, rsp2 []byte) float64 {
	rspIns1, err := lowhttp.ParseStringToHTTPResponse(string(rsp1))
	if err != nil {
		log.Errorf("parse string to response_1 failed: %s", err)
	}
	rspIns2, err := lowhttp.ParseStringToHTTPResponse(string(rsp2))
	if err != nil {
		log.Errorf("parse string to response_2 failed: %s", err)
	}

	if rspIns1 == nil || rspIns2 == nil {
		return compareBytes(rsp1, rsp2)
	}
	return CompareHTTPResponse(rspIns1, rspIns2)
}

// CompareHTTPResponse 比较两个 http.Response 对象的相似度，返回 0 到 1 之间的相似度分值
// 参数:
//   - rsp1: 第一个 http.Response 对象
//   - rsp2: 第二个 http.Response 对象
//
// 返回值:
//   - 相似度分值，1 表示完全相同，0 表示完全不同
//
// Example:
// ```
// // 比较两个 http.Response 对象的相似度(需先获得 Response 对象，作示意)
// score = judge.CompareHTTPResponse(rsp1, rsp2)
// println(score)
// ```
func CompareHTTPResponse(rsp1 *http.Response, rsp2 *http.Response) float64 {
	config := &Config{BodyLimit: 4096}

	if rsp1 == nil || rsp2 == nil {
		return 0
	}

	var scoreFloat64 score = 0

	// 设置 URL 权重
	var url1, url2 string
	url1Ins, _ := lowhttp.ExtractURLFromHTTPRequest(rsp1.Request, false)
	if url1Ins != nil {
		url1 = url1Ins.String()
	}
	url2Ins, _ := lowhttp.ExtractURLFromHTTPRequest(rsp2.Request, false)
	if url2Ins != nil {
		url2 = url2Ins.String()
	}
	if url1 == url2 && url1 == "" {
		scoreFloat64 = scoreFloat64.Add(1, 0.2)
	} else {
		scoreFloat64 = scoreFloat64.Add(compareString(url1, url2), 0.2)
	}

	// 设置状态权重
	status1, status2 := rsp1.StatusCode, rsp2.StatusCode
	scoreFloat64 = scoreFloat64.Add(eq(status1, status2), 0.3)

	// content-type
	// 判断是否是 JSON / XML(HTML)
	jsonCompareBody := false
	ct1, ct2 := rsp1.Header.Get("Content-Type"), rsp1.Header.Get("Content-Type")
	if utils.MatchAllOfRegexp(ct1, `(?i)json`) && utils.MatchAllOfRegexp(ct2, `(?i)json`) {
		jsonCompareBody = true
	}
	scoreFloat64 = scoreFloat64.Add(eq(ct1, ct2), 0.02)

	// set-cookie
	cookie1, cookie2 := rsp1.Cookies(), rsp2.Cookies()
	sort.Stable(CookieSortable(cookie2))
	sort.Stable(CookieSortable(cookie1))
	scoreFloat64 = scoreFloat64.Add(compareCookies(cookie1, cookie2), 0.02)

	// 整体 Header 相似度
	header1, _ := utils.DumpHTTPResponse(rsp1, false)
	header2, _ := utils.DumpHTTPResponse(rsp2, false)
	scoreFloat64 = scoreFloat64.Add(compareBytes(header1, header2), 0.06)

	// 读取并恢复 Body
	body1, err := io.ReadAll(rsp1.Body)
	if err != nil {
		log.Error(err)
	}
	rsp1.Body = ioutil.NopCloser(bytes.NewBuffer(body1))
	if len(body1) > config.BodyLimit {
		body1 = body1[:config.BodyLimit]
	}
	body2, err := ioutil.ReadAll(rsp2.Body)
	if err != nil {
		log.Error(err)
	}
	rsp2.Body = ioutil.NopCloser(bytes.NewBuffer(body2))
	if len(body2) > config.BodyLimit {
		body2 = body2[:config.BodyLimit]
	}

	if body1 == nil && body2 == nil {
		scoreFloat64 = scoreFloat64.Add(1, 0.4)
	} else {
		if jsonCompareBody {
			scoreFloat64 = scoreFloat64.Add(CompareJsons(body1, body2), 0.4)
		} else {
			scoreFloat64 = scoreFloat64.Add(CompareHtml(body1, body2), 0.4)
		}
	}

	return shrink(float64(scoreFloat64))
}
