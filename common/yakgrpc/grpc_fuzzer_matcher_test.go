package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"testing"
	"time"
)

func TestMatcher(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	first := true
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		if first {
			w.Write([]byte("abc"))
			first = false
		} else {
			w.Write([]byte("123"))
		}
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:     "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		ForceFuzz:   true,
		RepeatTimes: 10,
	})
	if err != nil {
		panic(err)
	}
	var taskID int64
	for i := 0; i < 10; i++ {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(resp.ResponseRaw)
		taskID = resp.TaskId
	}

	matcher := &ypb.HTTPResponseMatcher{
		MatcherType: "word",
		Scope:       "raw",
		Condition:   "and",
		Group:       []string{"123"},
		ExprType:    "nuclei-dsl",
	}

	matcherStream, err := client.MatchTaskResponse(context.Background(), &ypb.MatchTask{
		TaskID:            taskID,
		Matchers:          []*ypb.HTTPResponseMatcher{matcher},
		MatchersCondition: "and",
		HitColor:          "red",
	})
	if err != nil {
		panic(err)
	}
	var matcherCheckCount int
	for i := 0; i < 10; i++ {
		resp, err := matcherStream.Recv()
		if err != nil {
			break
		}
		if resp.MatchedByMatcher {
			matcherCheckCount++
		}
	}
	if matcherCheckCount != 9 {
		t.Fatalf("matcher check failed: need [%v] got [%v]", 9, matcherCheckCount)
	}
}
