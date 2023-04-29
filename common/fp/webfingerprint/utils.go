package webfingerprint

import (
	"net/url"
	"regexp"
	"strings"
)

import (
	"crypto/tls"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

func HttpInsecureGet(url string) (*http.Response, error) {
	client := http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		Timeout:   10 * time.Second,
	}
	return client.Get(url)
}

func HttpGet(url string) ([]byte, error) {

	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Errorf("HTTP GET %s error: %s", url, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Errorf("read response body error: %s", body)
	}
	return body, nil
}

func HttpGetWithProgress(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Errorf("HTTP GET %s error: %s", url, err)
	}
	defer resp.Body.Close()
	fileSize, err := strconv.ParseUint(resp.Header.Get("Content-Length"), 10, 64)
	counter := &WriteCounter{FileSize: fileSize}
	body, err := ioutil.ReadAll(io.TeeReader(resp.Body, counter))
	if err != nil {
		return nil, errors.Errorf("read response body error: %s", body)
	}
	return body, nil
}

// HTTPDownloadToFile download the resource to a specific file
func HTTPDownloadToFile(url string, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	return err
}

// WriteCounter counts the number of bytes written to it. It implements to the io.Writer
// interface and we can pass this into io.TeeReader() which will report progress on each
// write cycle.
type WriteCounter struct {
	Total    uint64
	FileSize uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc WriteCounter) PrintProgress() {
	fmt.Printf("\rDownloading... %f MB complete. %f %%  ",
		float64(wc.Total)/(1024*1024),
		float64(wc.Total)/float64(wc.FileSize)*100)
}

func HttpGetWithRetry(retry int, url string) ([]byte, error) {
	var e error
	for ; retry > 0; retry-- {
		b, err := HttpGet(url)
		if err == nil {
			return b, nil
		} else {
			e = err
			continue
		}
	}
	return nil, e
}

// map[string][]string to query
func MapQueryToString(values map[string][]string) string {
	var items []string
	for key, vals := range values {
		key = url.QueryEscape(key)
		for _, val := range vals {
			val = url.QueryEscape(val)

			var (
				item string
			)
			if key != "" {
				item = fmt.Sprintf("%s=%s", key, val)
			} else {
				item = val
			}

			if item != "" {
				items = append(items, item)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i] < items[j]
	})
	return strings.Join(items, "&")
}

func DomainToURLFilter(domain string) (*regexp.Regexp, error) {
	urlFilterTmp := "^https?://%s"
	var re string

	if !strings.Contains(domain, "/") {
		// 不包含路径匹配，那么通配符不应该包含路径和问号
		if !strings.Contains(domain, "*") {
			re = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(domain))
		} else {
			re = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(strings.ReplaceAll(domain, "*", "WILDCARD")))
			re = strings.ReplaceAll(re, "WILDCARD", "[^/^?^#]*")
		}
	} else {
		results := strings.SplitN(domain, "/", 2)
		if len(results) != 2 {
			return nil, errors.Errorf("[%s] split path failed", domain)
		}

		domain, path := results[0], results[1]
		var (
			domainRegex, pathRegex string
		)

		// 匹配域名部分
		if !strings.Contains(domain, "*") {
			domainRegex = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(domain))
		} else {
			domainRegex = fmt.Sprintf(urlFilterTmp, regexp.QuoteMeta(strings.ReplaceAll(domain, "*", "WILDCARD")))
			domainRegex = strings.ReplaceAll(domainRegex, "WILDCARD", "[^/^?^#^&]*")
		}

		// 匹配路径部分
		if strings.Contains(path, "*") {
			pathRegex = strings.ReplaceAll(path, "*", ".*")
		}

		re = fmt.Sprintf(`%s/%s`, domainRegex, pathRegex)
		// if re == "\\/" {
		//	log.Fatalf("domain: %s path: %s re: %s", domainRegex, pathRegex, re)
		// }
	}

	return regexp.Compile(re)
}

func CalcAbsolutelyURLWithoutFragment(origin *url.URL, u string) string {
	if strings.HasPrefix(u, "#") {
		return ""
	}

	absURL, err := origin.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = origin.Scheme
	}
	return absURL.String()
}

func CalcAbsolutelyURLWithFragment(origin *url.URL, u string) string {
	absURL, err := origin.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = origin.Scheme
	}
	return absURL.String()
}
