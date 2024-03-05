package yakurl

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"strings"
)

func CreateUrlFromString(raw string) (*ypb.YakURL, error) {
	if raw == "" {
		return nil, utils.Error("empty yak url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, utils.Errorf("cannot parse raw[%v] as url: %s", raw, err)
	}
	yu := &ypb.YakURL{
		Schema: strings.TrimSpace(strings.ToLower(u.Scheme)),
	}
	if u.User != nil {
		yu.User = u.User.Username()
		yu.Pass, _ = u.User.Password()
	}
	yu.Location = u.Host
	for k, v := range u.Query() {
		for _, v1 := range v {
			yu.Query = append(yu.Query, &ypb.KVPair{
				Key:   utils.EscapeInvalidUTF8Byte([]byte(k)),
				Value: utils.EscapeInvalidUTF8Byte([]byte(v1)),
			})
		}
	}

	yu.Path = utils.EscapeInvalidUTF8Byte([]byte(u.EscapedPath()))
	if len(yu.Path) >= 2 {
		if yu.Path[2] == ':' {
			yu.Path = strings.TrimPrefix(yu.Path, "/")
		}
	}
	return yu, nil
}

func LoadGetResource(u string) (*ypb.RequestYakURLResponse, error) {
	yu, err := CreateUrlFromString(u)
	if err != nil {
		return nil, err
	}
	return GetActionService().GetAction(yu.Schema).Get(&ypb.RequestYakURLParams{
		Method:   "get",
		Url:      yu,
		Page:     1,
		PageSize: 200,
	})
}
