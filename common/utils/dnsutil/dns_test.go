package dnsutil

import (
	"github.com/davecgh/go-spew/spew"
	"testing"
	"time"
)

func TestQueryIP(t *testing.T) {
	spew.Dump(QueryIP("baidu.com", 1*time.Second, nil))
	spew.Dump(QueryIPAll("baidu.com", 1*time.Second, nil))
	spew.Dump(QueryNS("baidu.com", 1*time.Second, nil))
	spew.Dump(QueryTxt("4dogs.cn", 1*time.Second, nil))
	spew.Dump(QueryAXFR("vulhub.org", 1*time.Second, []string{"127.0.0.1:53"}))
}
