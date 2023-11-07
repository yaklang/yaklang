package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/url"
	"path"
	"strings"
)

func ExtractorVarsFromPacket(packet []byte, isHttps bool) map[string]string {
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(packet, isHttps)
	if err != nil {
		log.Error(err)
		return nil
	}
	return ExtractorVarsFromUrl(urlIns.String())
}
func ExtractorVarsFromUrl(u string) map[string]string {
	urlIns, err := url.Parse(u)
	if err != nil {
		log.Error(err)
		return nil
	}
	baseUrl := urlIns.String()
	var rootUrl, port string
	if urlIns.Scheme == "https" {
		if urlIns.Port() == "443" || urlIns.Port() == "" {
			port = "443"
			rootUrl = fmt.Sprintf("https://%v", urlIns.Host)
		} else {
			port = urlIns.Port()
			rootUrl = fmt.Sprintf("https://%v", utils.HostPort(urlIns.Host, urlIns.Port()))
		}
	} else {
		if urlIns.Port() == "80" || urlIns.Port() == "" {
			port = "80"
			rootUrl = fmt.Sprintf("http://%v", urlIns.Host)
		} else {
			port = urlIns.Port()
			rootUrl = fmt.Sprintf("http://%v", utils.HostPort(urlIns.Host, urlIns.Port()))
		}
	}
	var file string
	pathRaw := urlIns.RequestURI()
	if strings.Contains(pathRaw, "?") {
		pathNoQuery := pathRaw[:strings.Index(pathRaw, "?")]
		_, file = path.Split(pathNoQuery)
	}
	baseUrl = strings.TrimRight(baseUrl, "/")
	rootUrl = strings.TrimRight(rootUrl, "/")
	return map[string]string{
		"Host":     urlIns.Hostname(),
		"Port":     port,
		"Hostname": urlIns.Host,
		"RootURL":  rootUrl,
		"BaseURL":  baseUrl,
		"Path":     urlIns.RequestURI(),
		"File":     file,
		"Schema":   urlIns.Scheme,
	}

}
