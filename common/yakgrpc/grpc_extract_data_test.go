package yakgrpc

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func TestExecExtractData(t *testing.T) {
	var result = map[string]string{
		"sdfasdf": `aa&%*(()+*&(_^&*)%^T($%&*(*(&%&^%^aabcasdfasdfasdfbbb*^(&*()_*&()_^*)^&%^&^%&*`,
		"&%*(()+*&(": `a


a&%*(()+*&(_^&*)%^

T($%&*(*(

&%&^%^aabcasdfasdfasdfbbb*^(&*()_*&()_^*)^&%^&^%&
*`,
		"(()+*&(_^&*)%^T($%&*(*(&%&^%^aabcasdfasdfasdfbbb*^(&*()_*&()_^*)^&%^&": `aa&%*(()+*&(_^&*)%^T($%&*(*(&%&^%^aabcasdfasdfasdfbbb*^(&*()_*&()_^*)^&%^&^%&*`,
	}
	for selected, data := range result {
		prefix, suffix, err := extractPrefixAndSuffix([]byte(data), []byte(selected), 20)
		if err != nil {
			panic(err)
		}
		spew.Dump(prefix, suffix)
		req, err := execExtractRequest(&ypb.ExtractDataRequest{
			Data:         []byte(data),
			Mode:         "regexp-between",
			PrefixRegexp: prefix,
			SuffixRegexp: suffix,
		})
		if err != nil {
			panic("extract request exec failed: " + err.Error())
		}
		if req != selected {
			panic("extract calc failed")
		} else {
			println(req)
		}
	}

}

func TestExtractBetween(t *testing.T) {

}
