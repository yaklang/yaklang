package hunter

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net/url"
)

type HunterResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Total int `json:"total"`
		Time  int `json:"time"`
		Arr   []struct {
			WebTitle     string `json:"web_title"`
			IP           string `json:"ip"`
			Port         int    `json:"port"`
			BaseProtocol string `json:"base_protocol"`
			Protocol     string `json:"protocol"`
			Domain       string `json:"domain"`
			Component    []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"component"`
			URL            string `json:"url"`
			Os             string `json:"os"`
			Country        string `json:"country"`
			Province       string `json:"province"`
			City           string `json:"city"`
			UpdatedAt      string `json:"updated_at"`
			StatusCode     int    `json:"status_code"`
			Number         string `json:"number"`
			Company        string `json:"company"`
			IsWeb          string `json:"is_web"`
			IsRisk         string `json:"is_risk"`
			IsRiskProtocol string `json:"is_risk_protocol"`
			AsOrg          string `json:"as_org"`
			Isp            string `json:"isp"`
			Banner         string `json:"banner"`
		} `json:"arr"`
		ConsumeQuota string `json:"consume_quota"`
		RestQuota    string `json:"rest_quota"`
	} `json:"data"`
}

var defaultHttpClient = utils.NewDefaultHTTPClient()

func HunterQuery(username, key string, query string, page, limit int) (*HunterResult, error) {
	values := make(url.Values)
	values.Set("username", username)
	values.Set("api-key", key)
	values.Set("search", base64.URLEncoding.EncodeToString([]byte(query)))
	values.Set("page", fmt.Sprint(page))
	values.Set("page_size", fmt.Sprint(limit))
	var res, err = defaultHttpClient.Get(fmt.Sprintf("https://hunter.qianxin.com/openApi/search?%s", values.Encode()))
	if err != nil {
		return nil, utils.Errorf("query hunter search api failed: %s", err)
	}
	raw, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}

	var result HunterResult
	err = json.Unmarshal(raw, &result)
	if err != nil {
		return nil, utils.Errorf("marshal hunter result failed: %s", err)
	}

	if result.Code != 200 {
		return &result, utils.Errorf("[%v]: %v", result.Code, result.Msg)
	}

	return &result, nil
}
