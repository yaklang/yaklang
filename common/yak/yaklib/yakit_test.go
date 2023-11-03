package yaklib

import (
	"encoding/hex"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-rod/rod/lib/utils"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestYakitServer_Addr(t *testing.T) {
	s := NewYakitServer(2335, SetYakitServer_ProgressHandler(func(id string, progress float64) {
		log.Infof("progress: %v  - percent float: %v", id, progress)
	}))
	go func() {
		s.Start()
	}()
	time.Sleep(time.Second * 1)

	spew.Dump(s.Addr())
	c := NewYakitClient(s.Addr())
	c.YakitSetProgressEx("test", 0.5)
	c.YakitSetProgressEx("test", 0.6)
	c.YakitSetProgressEx("test", 0.7)
	c.YakitSetProgressEx("test", 0.8)
	c.YakitSetProgressEx("test", 0.99)

	time.Sleep(time.Second)
}

func TestMUSTPASS_YakitLog(t *testing.T) {
	randStr := utils.RandString(20)
	testCases := []struct {
		name     string
		format   string
		input    string
		expected string
	}{
		{"Test1", "%%d %s", randStr, "%d " + randStr},
		{"Test2", "%s", randStr, randStr},
		{"Test3", "%x", randStr, hex.EncodeToString([]byte(randStr))},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			GetExtYakitLibByClient(NewVirtualYakitClient(func(i *ypb.ExecResult) error {
				bb := jsonpath.Find(i.GetMessage(), "$.content.data")
				if bb != tc.expected {
					t.Fatalf("expect: %s, got: %s", tc.expected, bb)
				}
				return nil
			}))["Info"].(func(tmp string, items ...interface{}))(tc.format, tc.input)
		})
	}
}
