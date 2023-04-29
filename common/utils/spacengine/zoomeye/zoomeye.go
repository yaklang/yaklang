package zoomeye

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"yaklang/common/utils"
)

type ZoomeyeResult struct {
	Code      int `json:"code"`
	Total     int `json:"total"`
	Available int `json:"available"`
	Matches   []struct {
		Rdns string `json:"rdns,omitempty"`
		Jarm string `json:"jarm"`
		Ico  struct {
			Mmh3 string `json:"mmh3"`
			Md5  string `json:"md5"`
		} `json:"ico"`
		Txtfile struct {
			Robotsmd5   string `json:"robotsmd5"`
			Securitymd5 string `json:"securitymd5"`
		} `json:"txtfile"`
		IP       string `json:"ip"`
		Portinfo struct {
			Hostname  string      `json:"hostname"`
			Os        string      `json:"os"`
			Port      int         `json:"port"`
			Service   string      `json:"service"`
			Title     interface{} `json:"title"`
			Version   string      `json:"version"`
			Device    string      `json:"device"`
			Extrainfo string      `json:"extrainfo"`
			Rdns      string      `json:"rdns"`
			App       string      `json:"app"`
			Banner    string      `json:"banner"`
		} `json:"portinfo"`
		Timestamp string `json:"timestamp"`
		Geoinfo   struct {
			Continent struct {
				Code  string `json:"code"`
				Names struct {
					En   string `json:"en"`
					ZhCN string `json:"zh-CN"`
				} `json:"names"`
				GeonameID interface{} `json:"geoname_id"`
			} `json:"continent"`
			Country struct {
				Code  string `json:"code"`
				Names struct {
					En   string `json:"en"`
					ZhCN string `json:"zh-CN"`
				} `json:"names"`
				GeonameID interface{} `json:"geoname_id"`
			} `json:"country"`
			BaseStation string `json:"base_station"`
			City        struct {
				Names struct {
					En   string `json:"en"`
					ZhCN string `json:"zh-CN"`
				} `json:"names"`
				GeonameID interface{} `json:"geoname_id"`
			} `json:"city"`
			Isp          string `json:"isp"`
			Organization string `json:"organization"`
			Idc          string `json:"idc"`
			Location     struct {
				Lon string `json:"lon"`
				Lat string `json:"lat"`
			} `json:"location"`
			Aso          interface{} `json:"aso"`
			Asn          string      `json:"asn"`
			Subdivisions struct {
				Names struct {
					En   string `json:"en"`
					ZhCN string `json:"zh-CN"`
				} `json:"names"`
				GeonameID interface{} `json:"geoname_id"`
			} `json:"subdivisions"`
			PoweredBy string `json:"PoweredBy"`
			Scene     struct {
				En string `json:"en"`
				Cn string `json:"cn"`
			} `json:"scene"`
			OrganizationCN interface{} `json:"organization_CN"`
		} `json:"geoinfo"`
		Protocol struct {
			Application string `json:"application"`
			Probe       string `json:"probe"`
			Transport   string `json:"transport"`
		} `json:"protocol"`
		Honeypot int         `json:"honeypot"`
		Whois    interface{} `json:"whois"`
	} `json:"matches"`
	Facets struct {
		Product []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"product"`
		Os []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"os"`
	} `json:"facets"`
}

var defaultHttpClient = utils.NewDefaultHTTPClient()

func ZoomeyeQuery(key string, query string, page int) (*ZoomeyeResult, error) {
	values := make(url.Values)
	values.Set("query", query)
	values.Set("page", fmt.Sprint(page))
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.zoomeye.org/host/search?%s", values.Encode()), nil)
	if err != nil {
		return nil, utils.Errorf("new request failed: %s", err)
	}
	req.Header.Set("API-KEY", key)

	res, err := defaultHttpClient.Do(req)
	if err != nil {
		return nil, utils.Errorf("query zoomeye search api failed: %s", err)
	}
	raw, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}

	var result ZoomeyeResult
	err = json.Unmarshal(raw, &result)
	if err != nil {
		return nil, utils.Errorf("marshal zoomeye result failed: %s", err)
	}

	if res.StatusCode != 200 {
		return &result, utils.Errorf("[%v]: invalid status code", result.Code)
	}

	return &result, nil
}
