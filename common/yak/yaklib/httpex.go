package yaklib

import (
	"bytes"
	"encoding/base64"
	"fmt"
	twmbMMH3 "github.com/twmb/murmur3"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"hash"
	"io/ioutil"
	"net/http"
	"time"
)

func requestToMd5(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Md5(raw), nil
}

func requestToSha1(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha1(raw), nil
}

func requestToSha256(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha256(raw), nil
}

func requestToSha512(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha512(raw), nil
}

func requestToMMH3Hash128(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.MMH3Hash128(raw), nil
}

func requestToMMH3Hash128x64(url string) (string, error) {
	rsp, err := netx.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.MMH3Hash128x64(raw), nil
}

func Mmh3Hash32(raw []byte) string {
	var h32 hash.Hash32 = twmbMMH3.New32()
	_, err := h32.Write([]byte(raw))
	if err == nil {
		return fmt.Sprintf("%d", int32(h32.Sum32()))
	} else {
		//log.Println("favicon Mmh3Hash32 error:", err)
		return "0"
	}
}

func CalcFaviconHash(urlRaw string) (string, error) {
	timeout := time.Duration(8 * time.Second)
	tr := netx.NewDefaultHTTPTransport()
	client := http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse /* 不进入重定向 */
		},
	}
	resp, err := client.Get(urlRaw)
	if err != nil {
		//log.Println("favicon client error:", err)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			//log.Println("favicon file read error: ", err)
			return "", err
		}
		return Mmh3Hash32(standBase64(body)), nil
	} else {
		return "", utils.Errorf("status code: %v", resp.StatusCode)
	}
}

func standBase64(braw []byte) []byte {
	bckd := base64.StdEncoding.EncodeToString(braw)
	var buffer bytes.Buffer
	for i := 0; i < len(bckd); i++ {
		ch := bckd[i]
		buffer.WriteByte(ch)
		if (i+1)%76 == 0 {
			buffer.WriteByte('\n')
		}
	}
	buffer.WriteByte('\n')
	return buffer.Bytes()
}

func init() {
	HttpExports["RequestFaviconHash"] = CalcFaviconHash
	HttpExports["RequestToMD5"] = requestToMd5
	HttpExports["RequestToSha1"] = requestToSha1
	HttpExports["RequestToSha256"] = requestToSha256
	HttpExports["RequestToMMH3Hash128"] = requestToMMH3Hash128
	HttpExports["RequestToMMH3Hash128x64"] = requestToMMH3Hash128x64
	HttpExports["RequestToSha256"] = requestToSha256
}
