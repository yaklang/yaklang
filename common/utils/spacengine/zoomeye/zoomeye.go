package zoomeye

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
)

func ZoomeyeQuery(key string, query string, page int) (*gjson.Result, error) {
	values := make(url.Values)
	values.Set("query", query)
	values.Set("page", fmt.Sprint(page))
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.zoomeye.org/host/search?%s", values.Encode()), nil)
	if err != nil {
		return nil, utils.Errorf("new request failed: %s", err)
	}
	req.Header.Set("API-KEY", key)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, utils.Errorf("query zoomeye search api failed: %s", err)
	}
	raw, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}

	result := gjson.ParseBytes(raw)
	if res.StatusCode != 200 {
		return &result, utils.Errorf("[%v]: invalid status code", res.StatusCode)
	}

	if !result.Get("matches").Exists() {
		return nil, utils.Errorf("no matches found")
	}

	return &result, nil
}
