package utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/quakeschema"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
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
	PaginationId string `json:"pagination_id,omitempty"`
	Query        string `json:"query"`
	Size         int    `json:"size"`
}

type quakeResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Meta    struct {
		Total        uint64 `json:"total"`
		PaginationId string `json:"pagination_id"`
	}
}

type quakeService struct {
	IP         string                   `json:"ip"`
	Port       int                      `json:"port"`
	IsIPv6     bool                     `json:"is_ipv6"`
	Asn        interface{}              `json:"asn"`
	Service    quakeServiceDetail       `json:"service"`
	Hostname   string                   `json:"hostname"`
	Transport  string                   `json:"transport"`
	Org        string                   `json:"org"`
	OsVersion  string                   `json:"os_version"`
	OsName     string                   `json:"os_name"`
	Time       string                   `json:"time"`
	Components []map[string]interface{} `json:"components"`
	Images     []interface{}            `json:"images"`
	Location   quakeLocation            `json:"location"`
}

type quakeServiceDetail struct {
	Tls      interface{} `json:"tls"`
	Http     quakeHttp   `json:"http"`
	Version  string      `json:"version"`
	Response string      `json:"response"`
	Product  string      `json:"product"`
	Name     string      `json:"name"`
}

type quakeHttp struct {
	Host         string `json:"host"`
	Server       string `json:"server"`
	Title        string `json:"title"`
	MetaKeywords string `json:"meta_keywords"`
	Body         string `json:"body"`
	StatusCode   int    `json:"status_code"`
}

type quakeLocation struct {
	Gps         []float64 `json:"gps"`
	CountryCode string    `json:"country_code"`
	CityCn      string    `json:"city_cn"`
	CityEn      string    `json:"city_en"`
	DistrictEn  string    `json:"district_en"`
	DistrictCn  string    `json:"district_cn"`
	Owner       string    `json:"owner"`
	ProvinceEn  string    `json:"province_en"`
	ProvinceCn  string    `json:"province_cn"`
	CountryEn   string    `json:"country_en"`
	CountryCn   string    `json:"country_cn"`
	ISP         string    `json:"isp"`
	StreetCn    string    `json:"street_cn"`
	StreetEn    string    `json:"street_en"`
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
	/*

		(map[string]interface {}) (len=4) {
		 (string) (len=4) "data": (map[string]interface {}) (len=17) {
		  (string) (len=2) "id": (string) (len=24) "60ed7abd26abc80dc60de79c",
		  (string) (len=5) "baned": (bool) false,
		  (string) (len=22) "month_remaining_credit": (float64) 38779,
		  (string) (len=27) "personal_information_status": (bool) false,
		  (string) (len=12) "total_credit": (float64) 38779,
		  (string) (len=15) "constant_credit": (float64) 0,
		  (string) (len=5) "token": (string) (len=36) "56867454-7cff-49e9-8484-2640f3abe9c6",
		  (string) (len=12) "disable_time": (interface {}) <nil>,
		  (string) (len=10) "ban_status": (string) (len=9) "使用中",
		  (string) (len=12) "mobile_phone": (string) (len=14) "+8618210074423",
		  (string) (len=6) "source": (string) (len=11) "360_account",
		  (string) (len=4) "time": (string) (len=19) "2021-07-13 19:36:29",
		  (string) (len=4) "user": (map[string]interface {}) (len=4) {
		   (string) (len=2) "id": (string) (len=24) "60ed7abd26abc80dc60de799",
		   (string) (len=8) "username": (string) (len=12) "debinli_naga",
		   (string) (len=8) "fullname": (string) (len=12) "debinli_naga",
		   (string) (len=5) "email": (interface {}) <nil>
		  },
		  (string) (len=9) "avatar_id": (string) (len=24) "60ed7abd26abc80dc60de79a",
		  (string) (len=11) "privacy_log": (map[string]interface {}) (len=2) {
		   (string) (len=6) "status": (bool) false,
		   (string) (len=4) "time": (interface {}) <nil>
		  },
		  (string) (len=22) "enterprise_information": (map[string]interface {}) (len=3) {
		   (string) (len=4) "name": (interface {}) <nil>,
		   (string) (len=5) "email": (interface {}) <nil>,
		   (string) (len=6) "status": (string) (len=9) "未认证"
		  },
		  (string) (len=4) "role": ([]interface {}) (len=2 cap=2) {
		   (map[string]interface {}) (len=3) {
		    (string) (len=6) "credit": (float64) 3000,
		    (string) (len=8) "fullname": (string) (len=12) "注册用户",
		    (string) (len=8) "priority": (float64) 4
		   },
		   (map[string]interface {}) (len=3) {
		    (string) (len=8) "fullname": (string) (len=12) "终身会员",
		    (string) (len=8) "priority": (float64) 9,
		    (string) (len=6) "credit": (float64) 50000
		   }
		  }
		 },
		 (string) (len=4) "meta": (map[string]interface {}) {
		 },
		 (string) (len=4) "code": (float64) 0,
		 (string) (len=7) "message": (string) (len=11) "Successful."
		}
	*/
	req, err := http.NewRequest("GET", quakeUserAPI, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QuakeToken", q.key)

	Debug(func() {
		reqRaw, err := httputil.DumpRequestOut(req, true)
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

	var quakeRsp quakeResponse
	err = json.Unmarshal(rspBody, &quakeRsp)
	if err != nil {
		Debug(func() {
			log.Infof("results: \n%v", string(rspBody))
		})
		if len(rspBody) > 101 {
			rspBody = append(rspBody[:97], '.', '.', '.')
		}
		return nil, Errorf("unmarshal [%v] failed: %v", string(rspBody), err)
	}

	if quakeRsp.Code != 0 {
		return nil, Errorf("quake error: %s", quakeRsp.Message)
	}

	var user *quakeUserInfo
	err = json.Unmarshal(quakeRsp.Data, &user)
	if err != nil {
		return nil, Errorf("marshal service failed: %s", err)
	}

	return user, nil
}

func (q *Quake360Client) QueryNext(queries ...string) ([]quakeschema.Data, error) {
	query := strings.Join(queries, " ")
	if query != "" && q.currentQuery != "" && q.currentQuery != query {
		return nil, Errorf("query empty or query changed from initd query")
	}

	if query == "" {
		return nil, Errorf("empty query")
	}

	q.currentQuery = query
	raw, err := json.Marshal(quakeQueryParam{
		PaginationId: q.currentPaginationId,
		Query:        query,
		Size:         10,
	})

	req, err := http.NewRequest("POST", quakeAPI, bytes.NewBuffer(raw))
	if err != nil {
		return nil, Errorf("create quake request failed: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QuakeToken", q.key)
	req.Header.Set("User-Agent", "curl/7.64.1")

	Debug(func() {
		reqRaw, err := httputil.DumpRequestOut(req, true)
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

	var quakeRsp quakeschema.QuakeResult
	err = json.Unmarshal(rspBody, &quakeRsp)
	if err != nil {
		Debug(func() {
			log.Infof("results: \n%v", string(rspBody))
		})
		if len(rspBody) > 101 {
			rspBody = append(rspBody[:97], '.', '.', '.')
		}
		return nil, Errorf("unmarshal [%v] failed: %v", string(rspBody), err)
	}

	if quakeRsp.Code != 0 {
		return nil, Errorf("quake error: %s", quakeRsp.Message)
	}

	//var services []*quakeService
	//err = json.Unmarshal(quakeRsp.Data, &services)
	//if err != nil {
	//	return nil, Errorf("marshal service failed: %s", err)
	//}

	q.currentPaginationId = quakeRsp.Meta.PaginationID
	q.currentQuery = query

	if len(quakeRsp.Data) <= 0 {
		return nil, Errorf("empty services / results")
	}

	return quakeRsp.Data, nil
}
