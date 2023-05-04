package comparer

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
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
	header1, _ := httputil.DumpResponse(rsp1, false)
	header2, _ := httputil.DumpResponse(rsp2, false)
	scoreFloat64 = scoreFloat64.Add(compareBytes(header1, header2), 0.06)

	// 读取并恢复 Body
	body1, err := ioutil.ReadAll(rsp1.Body)
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
