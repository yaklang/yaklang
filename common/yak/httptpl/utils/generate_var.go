package utils

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
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
	rootUrl := ""
	hostname, port := urlIns.Hostname(), urlIns.Port()
	if urlIns.Scheme == "https" {
		if port == "443" || port == "" {
			port = "443"
			rootUrl = fmt.Sprintf("https://%v", hostname)
		} else {
			rootUrl = fmt.Sprintf("https://%v", urlIns.Host)
		}
	} else {
		if port == "80" || port == "" {
			port = "80"
			rootUrl = fmt.Sprintf("http://%v", hostname)
		} else {
			rootUrl = fmt.Sprintf("http://%v", urlIns.Host)
		}
	}
	var file string
	pathRaw := urlIns.RequestURI()
	if strings.Contains(pathRaw, "?") {
		pathRaw = pathRaw[:strings.Index(pathRaw, "?")]
	}
	_, file = path.Split(pathRaw)
	baseUrl = strings.TrimRight(baseUrl, "/")
	rootUrl = strings.TrimRight(rootUrl, "/")
	return map[string]string{
		"Host":     hostname,
		"Port":     port,
		"Hostname": urlIns.Host,
		"RootURL":  rootUrl,
		"BaseURL":  baseUrl,
		"Path":     urlIns.RequestURI(),
		"File":     file,
		"Schema":   urlIns.Scheme,
	}
}
