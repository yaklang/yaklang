package yakgrpc

import (
	"context"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_HTTPFuzzer_ReMatcher(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var mu sync.Mutex
	first := true
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {

		if first {
			mu.Lock()
			if first {
				w.Write([]byte("abc"))
				first = false
			} else {
				w.Write([]byte("123"))
			}
			mu.Unlock()
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

	matcherStream, err := client.HTTPFuzzer(context.Background(),
		&ypb.FuzzerRequest{
			Matchers:           []*ypb.HTTPResponseMatcher{matcher},
			HistoryWebFuzzerId: int32(taskID),
			ReMatch:            true,
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

func TestGRPCMUSTPASS_HTTPFuzzer_ReMatcherWithParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var mu sync.Mutex
	first := true
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(w http.ResponseWriter, r *http.Request) {
		if first {
			mu.Lock()
			if first {
				w.Write([]byte("abc"))
				first = false
			} else {
				w.Header().Add("test", "123")
				w.Write([]byte("123"))
			}
			mu.Unlock()
		} else {
			w.Header().Add("test", "123")
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
		//spew.Dump(resp.ResponseRaw)
		taskID = resp.TaskId
	}

	matcher := &ypb.HTTPResponseMatcher{
		MatcherType: "expr",
		Scope:       "raw",
		Condition:   "and",
		Group:       []string{"contains(body,fuzzParam)", "extractParam == fuzzParam"},
		ExprType:    "nuclei-dsl",
	}

	extractor := &ypb.HTTPResponseExtractor{Scope: "header", Groups: []string{"Test"}, Type: "kval", Name: "extractParam"}

	fuzzParam := &ypb.FuzzerParamItem{
		Key:   "fuzzParam",
		Value: "123",
	}

	err = utils.AttemptWithDelayFast(func() error {
		matcherStream, err := client.HTTPFuzzer(context.Background(),
			&ypb.FuzzerRequest{
				Matchers:           []*ypb.HTTPResponseMatcher{matcher},
				Extractors:         []*ypb.HTTPResponseExtractor{extractor},
				HistoryWebFuzzerId: int32(taskID),
				ReMatch:            true,
				Params:             []*ypb.FuzzerParamItem{fuzzParam},
			})

		if err != nil {
			return err
		}
		var matcherCheckCount int
		for i := 0; i < 10; i++ {
			resp, err := matcherStream.Recv()
			if err != nil {
				log.Errorf("err: %v", err)
				break
			}
			spew.Dump(resp)
			if resp.MatchedByMatcher {
				matcherCheckCount++
			}
		}
		if matcherCheckCount != 9 {
			return utils.Errorf("matcher check failed: need [%v] got [%v]", 9, matcherCheckCount)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestFuzzerExtractorInvalidUTF8(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nTest: \xff\xff\r\n\r\nabc"))
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	target := utils.HostPort(host, port)

	extractor := &ypb.HTTPResponseExtractor{Scope: "header", Groups: []string{"Test"}, Type: "kval", Name: "extractParam"}

	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:    "GET / HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		ForceFuzz:  true,
		Extractors: []*ypb.HTTPResponseExtractor{extractor},
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			} else {
				t.Fatal(err)
			}
		}
		spew.Dump(rsp)
	}
}
