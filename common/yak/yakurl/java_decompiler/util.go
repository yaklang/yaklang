package java_decompiler

import (
	"fmt"
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
		schema, rawPath, queryStr string
		isWindowsPath             bool
	)

	schema, rawPath, _ = strings.Cut(raw, "://")
	// should not use filepath.VolumeName
	if ret := windowsPathPrefixRegex.FindStringSubmatch(rawPath); len(ret) > 1 && ret[0] != "" {
		// maybe windows path, fix
		isWindowsPath = true
		// file://C:\\A\B\C?q=C:\\A\B\C
		rawPath, queryStr, _ = strings.Cut(rawPath, "?")
		raw = fmt.Sprintf("%s://%s?%s", schema, windowsPathPrefixRegex.ReplaceAllString(strings.ReplaceAll(rawPath, "\\", "/"), "/"), queryStr)
	}

	u := utils.ParseStringToUrl(raw)

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
	if len(yu.Path) > 2 {
		if yu.Path[2] == ':' {
			yu.Path = strings.TrimPrefix(yu.Path, "/")
		}
	}
	if isWindowsPath {
		yu.Path = rawPath
	}
	return yu, nil
}
