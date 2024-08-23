package yakgrpc

import (
	"context"
	"errors"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	token := utils.RandStringBytes(10)
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:     "GET /?token=" + token + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
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

	_, err = QueryHTTPFlows(utils.TimeoutContextSeconds(2), client, &ypb.QueryHTTPFlowRequest{
		Keyword: token,
	}, 10)
	require.NoError(t, err)

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
		spew.Dump(resp)
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
	require.NoError(t, utils.AttemptWithDelayFast(func() error {
		taskRespCount, err := yakit.CountWebFuzzerResponses(consts.GetGormProjectDatabase(), int(taskID))
		if err != nil {
			return err
		}
		if taskRespCount != 10 {
			return utils.Errorf("want 10 task resp ,but got %d", taskRespCount)
		}
		return nil
	}))

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

func TestFuzzerMatchMultipleColor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	token1 := utils.RandStringBytes(5)
	token2 := utils.RandStringBytes(5)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		index := codec.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
		if index%2 == 0 {
			return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token1)
		} else {
			return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token2)
		}
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	matcher1 := &ypb.HTTPResponseMatcher{
		MatcherType: "word",
		Scope:       "raw",
		Condition:   "and",
		Group:       []string{token1},
		ExprType:    "nuclei-dsl",
		HitColor:    "red",
	}
	matcher2 := &ypb.HTTPResponseMatcher{
		MatcherType: "word",
		Scope:       "raw",
		Condition:   "and",
		Group:       []string{token2},
		ExprType:    "nuclei-dsl",
		HitColor:    "blue",
	}
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:   "GET /?a={{i(0-10)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		ForceFuzz: true,
		Matchers:  []*ypb.HTTPResponseMatcher{matcher1, matcher2},
	})
	require.NoError(t, err)
	var redCount int
	var blueCount int
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp.MatchedByMatcher {
			if resp.HitColor == "red" {
				redCount++
			}
			if resp.HitColor == "blue" {
				blueCount++
			}
		}
	}
	require.Equal(t, 6, redCount, "token1 count is not 6")
	require.Equal(t, 5, blueCount, "token2 count is not 5")
}

func TestFuzzerMatchMultipleColor_HasSubMatcher(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	token1 := utils.RandStringBytes(5)
	token2 := utils.RandStringBytes(5)
	token3 := utils.RandStringBytes(5)
	token4 := utils.RandStringBytes(5)
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		index := codec.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
		if index == 0 {
			return []byte("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\n" + token1 + token3)
		} else if index == 1 {
			return []byte("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\n" + token2 + token4)
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
	})

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	matcher1 := &ypb.HTTPResponseMatcher{
		SubMatcherCondition: "and",
		SubMatchers: []*ypb.HTTPResponseMatcher{
			{
				MatcherType: "word",
				Scope:       "raw",
				Condition:   "and",
				Group:       []string{token1},
				ExprType:    "nuclei-dsl",
			},
			{
				MatcherType: "word",
				Scope:       "raw",
				Condition:   "and",
				Group:       []string{token3},
				ExprType:    "nuclei-dsl",
			},
		},
		HitColor: "green",
	}
	matcher2 := &ypb.HTTPResponseMatcher{
		MatcherType: "word",
		Scope:       "raw",
		Condition:   "and",
		Group:       []string{token2, token4},
		ExprType:    "nuclei-dsl",
		HitColor:    "blue",
	}
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:   "GET /?a={{i(0-1)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
		ForceFuzz: true,
		Matchers:  []*ypb.HTTPResponseMatcher{matcher1, matcher2},
	})
	require.NoError(t, err)
	var greenCount int
	var blueCount int
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp.MatchedByMatcher {
			if resp.HitColor == "green" {
				greenCount++
			}
			if resp.HitColor == "blue" {
				blueCount++
			}
		}
	}
	require.Equal(t, 1, greenCount, "green count is not 1")
	require.Equal(t, 1, blueCount, "blue count is not 1")
}

func TestFuzzerMatchMultipleAction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("retain test", func(t *testing.T) {
		token1 := utils.RandStringBytes(5)
		token2 := utils.RandStringBytes(5)
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			index := codec.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
			if index%2 == 0 {
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token1)
			} else {
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token2)
			}
		})
		matcher1 := &ypb.HTTPResponseMatcher{
			MatcherType: "word",
			Scope:       "raw",
			Condition:   "and",
			Group:       []string{token1},
			ExprType:    "nuclei-dsl",
			HitColor:    "red",
			Action:      "retain",
		}

		matcher2 := &ypb.HTTPResponseMatcher{
			MatcherType: "word",
			Scope:       "raw",
			Condition:   "and",
			Group:       []string{token2},
			ExprType:    "nuclei-dsl",
			HitColor:    "blue",
		}

		target := utils.HostPort(host, port)
		stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request:   "GET /?a={{i(0-10)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
			ForceFuzz: true,
			Matchers:  []*ypb.HTTPResponseMatcher{matcher1, matcher2},
		})
		require.NoError(t, err)
		var allCount int
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if resp.MatchedByMatcher {
				require.Equal(t, "red", resp.HitColor, "retain action return color is not red")
				allCount++
			}
		}
		require.Equal(t, 6, allCount, "retain all count is not 6")
	})

	t.Run("discard test", func(t *testing.T) {
		token1 := utils.RandStringBytes(5)
		token2 := utils.RandStringBytes(5)
		host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
			index := codec.Atoi(lowhttp.GetHTTPRequestQueryParam(req, "a"))
			if index%2 == 0 {
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token1)
			} else {
				return []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" + token2)
			}
		})
		matcher1 := &ypb.HTTPResponseMatcher{
			MatcherType: "word",
			Scope:       "raw",
			Condition:   "and",
			Group:       []string{token1},
			ExprType:    "nuclei-dsl",
			HitColor:    "red",
			Action:      "discard",
		}

		matcher2 := &ypb.HTTPResponseMatcher{
			MatcherType: "word",
			Scope:       "raw",
			Condition:   "and",
			Group:       []string{token2},
			ExprType:    "nuclei-dsl",
			HitColor:    "blue",
		}

		target := utils.HostPort(host, port)
		stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
			Request:   "GET /?a={{i(0-10)}} HTTP/1.1\r\nHost: " + target + "\r\n\r\n",
			ForceFuzz: true,
			Matchers:  []*ypb.HTTPResponseMatcher{matcher1, matcher2},
		})
		require.NoError(t, err)
		var allCount int
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if resp.MatchedByMatcher {
				require.Equal(t, "blue", resp.HitColor, "discard action return color is not blue")
				allCount++
			}
		}
		require.Equal(t, 5, allCount, "discard all count is not 5")
	})
}
