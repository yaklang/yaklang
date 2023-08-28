package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/log"
)

func newDefaultClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
				MaxVersion:         tls.VersionTLS13,
			},
			DisableKeepAlives:  true,
			DisableCompression: true,
			MaxConnsPerHost:    50,
			Proxy:              nil,
		},
		Timeout: 15 * time.Second,
	}
}

type Quake360Client struct {
	key                 string
	currentPaginationId string
	currentQuery        string
	client              *http.Client
}

func NewQuake360Client(apiKey string) *Quake360Client {
	return &Quake360Client{key: apiKey, client: newDefaultClient()}
}

const quakeAPI = "https://quake.360.cn/api/v3/scroll/quake_service"
const quakeUserAPI = "https://quake.360.cn/api/v3/user/info"

type quakeQueryParam struct {
	Query string `json:"query"`
	Start int    `json:"start"`
	Size  int    `json:"size"`
}

type quakeUserInfo struct {
	Id      string `json:"id"`
	IsBaned bool   `json:"baned"`
	// 当月剩余
	MonthRemainingCredit int    `json:"month_remaining_credit"`
	TotalCredit          int    `json:"total_credit"`
	ConstantCredit       int    `json:"constant_credit"`
	BanStatus            string `json:"ban_status"`
}

func (q *Quake360Client) UserInfo() (*quakeUserInfo, error) {
	req, err := http.NewRequest("GET", quakeUserAPI, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QuakeToken", q.key)
	req.Header.Set("User-Agent", "curl/7.64.1")

	Debug(func() {
		reqRaw, err := DumpHTTPRequest(req, true)
		if err != nil {
			return
		}
		log.Infof("req: \n%s", string(reqRaw))
	})

	rsp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}

	rspBody, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}

	result := gjson.ParseBytes(rspBody)
	Debug(func() {
		log.Infof("results: \n%v", string(rspBody))
	})

	code := result.Get("code")
	if !code.Exists() || code.Int() != 0 {
		return nil, Errorf("quake error: %s", result.Get("message").String())
	}

	data := result.Get("data")
	user := &quakeUserInfo{
		Id:                   data.Get("id").String(),
		IsBaned:              data.Get("baned").Bool(),
		MonthRemainingCredit: int(data.Get("month_remaining_credit").Int()),
		TotalCredit:          int(data.Get("total_credit").Int()),
		ConstantCredit:       int(data.Get("constant_credit").Int()),
		BanStatus:            data.Get("ban_status").String(),
	}

	return user, nil
}

func (q *Quake360Client) QueryNext(start, size int, queries ...string) (*gjson.Result, error) {
	query := strings.Join(queries, " ")
	if query != "" && q.currentQuery != "" && q.currentQuery != query {
		return nil, Errorf("query empty or query changed from initd query")
	}

	if query == "" {
		return nil, Errorf("empty query")
	}

	q.currentQuery = query
	raw, err := json.Marshal(quakeQueryParam{
		Query: query,
		Start: start,
		Size:  size,
	})

	req, err := http.NewRequest("POST", quakeAPI, bytes.NewBuffer(raw))
	if err != nil {
		return nil, Errorf("create quake request failed: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QuakeToken", q.key)
	req.Header.Set("User-Agent", "curl/7.64.1")

	Debug(func() {
		reqRaw, err := DumpHTTPRequest(req, true)
		if err != nil {
			return
		}
		log.Infof("req: \n%s", string(reqRaw))
	})

	rsp, err := q.client.Do(req)
	if err != nil {
		return nil, Errorf("quake request from http client failed: %s", err)
	}
	if rsp.Body == nil {
		return nil, Errorf("empty result")
	}

	rspBody, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, Errorf("empty result")
	}
	rspStr := string(rspBody)

	result := gjson.Parse(rspStr)
	Debug(func() {
		log.Infof("results: \n%v", rspStr)
	})

	code := result.Get("code")
	if !code.Exists() || code.Int() != 0 {
		return nil, Errorf("quake error: %s", result.Get("message").String())
	}

	q.currentQuery = query
	dataArray := result.Get("data").Array()
	if len(dataArray) <= 0 {
		return nil, Errorf("empty services / results")
	}

	return &result, nil
}
