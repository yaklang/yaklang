package yakurl

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var windowsPathPrefixRegex = regexp.MustCompile(`([a-zA-Z]):(\\|\/)`)

func CreateUrlFromString(raw string) (*ypb.YakURL, error) {
	if raw == "" {
		return nil, utils.Error("empty yak url")
	}
	var (
		isWindowsPath = false
		volumeName    string
	)

	before, rawPath, _ := strings.Cut(raw, "://")
	if ret := windowsPathPrefixRegex.FindStringSubmatch(rawPath); len(ret) > 1 && ret[0] != "" {
		// maybe windows path
		isWindowsPath = true
		volumeName = ret[1] + ":"
		// replace windows path prefix, to avoid url.Parse error
		raw = before + "://" + windowsPathPrefixRegex.ReplaceAllString(strings.ReplaceAll(rawPath, "\\", "/"), "/")
	}
	_ = isWindowsPath

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
	if isWindowsPath {
		yu.Path = volumeName + strings.ReplaceAll(yu.Path, "/", "\\")
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
