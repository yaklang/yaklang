package hunter

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/utils"
)

func HunterQuery(username, key, query string, page, pageSize int) (*gjson.Result, error) {
	values := make(url.Values)
	values.Set("username", username)
	values.Set("api-key", key)
	values.Set("search", base64.URLEncoding.EncodeToString([]byte(query)))
	values.Set("page", fmt.Sprint(page))
	values.Set("page_size", fmt.Sprint(pageSize))
	var res, err = http.DefaultClient.Get(fmt.Sprintf("https://hunter.qianxin.com/openApi/search?%s", values.Encode()))
	if err != nil {
		return nil, utils.Errorf("query hunter search api failed: %s", err)
	}
	raw, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, utils.Errorf("read body failed: %s", err)
	}

	result := gjson.ParseBytes(raw)
	code, errmsg := result.Get("code").Int(), result.Get("msg").String()

	if result.Get("code").Int() != 200 {
		return &result, utils.Errorf("[%v]: %v", code, errmsg)
	}

	return &result, nil
}
