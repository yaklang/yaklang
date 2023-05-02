package yaklib

import (
	"io/ioutil"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib/codec"
)

func requestToMd5(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Md5(raw), nil
}

func requestToSha1(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha1(raw), nil
}

func requestToSha256(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha256(raw), nil
}

func requestToSha512(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.Sha512(raw), nil
}

func requestToMMH3Hash128(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.MMH3Hash128(raw), nil
}

func requestToMMH3Hash128x64(url string) (string, error) {
	rsp, err := utils.NewDefaultHTTPClient().Get(url)
	if err != nil {
		return "", err
	}
	raw, _ := ioutil.ReadAll(rsp.Body)
	return codec.MMH3Hash128x64(raw), nil
}

func init() {
	HttpExports["RequestFaviconHash"] = utils.CalcFaviconHash
	HttpExports["RequestToMD5"] = requestToMd5
	HttpExports["RequestToSha1"] = requestToSha1
	HttpExports["RequestToSha256"] = requestToSha256
	HttpExports["RequestToMMH3Hash128"] = requestToMMH3Hash128
	HttpExports["RequestToMMH3Hash128x64"] = requestToMMH3Hash128x64
	HttpExports["RequestToSha256"] = requestToSha256
}
