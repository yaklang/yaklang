package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRedirectRequestCookieAndRequestSemantics(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("same origin keeps original login cookie", func(t *testing.T) {
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodGet || len(body) != 0 || request.Header.Get("Cookie") != "PHPSESSID=original" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Bingo", "manual-cookie-kept")
			writer.WriteHeader(http.StatusOK)
		})

		target := utils.HostPort(host, port)
		result, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			Request:  "POST /login.php HTTP/1.1\r\nHost: " + target + "\r\nCookie: PHPSESSID=original\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 3\r\n\r\na=b",
			Response: "HTTP/1.1 302 Found\r\nLocation: main.php\r\n\r\n",
		})
		require.NoError(t, err)
		require.Equal(t, int32(http.StatusOK), result.GetStatusCode())
		require.Equal(t, http.MethodGet, lowhttp.GetHTTPRequestMethod(result.GetRequestRaw()))
		require.Equal(t, "PHPSESSID=original", lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "Cookie"))
		require.Empty(t, lowhttp.GetHTTPPacketBody(result.GetRequestRaw()))
	})

	t.Run("response cookie overrides original cookie", func(t *testing.T) {
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Cookie") != "PHPSESSID=rotated" {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			writer.Header().Set("Bingo", "manual-cookie-rotated")
			writer.WriteHeader(http.StatusOK)
		})

		target := utils.HostPort(host, port)
		result, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			Request:  "POST /login.php HTTP/1.1\r\nHost: " + target + "\r\nCookie: PHPSESSID=original\r\nContent-Length: 3\r\n\r\na=b",
			Response: "HTTP/1.1 302 Found\r\nLocation: main.php\r\nSet-Cookie: PHPSESSID=rotated; Path=/; HttpOnly\r\n\r\n",
		})
		require.NoError(t, err)
		require.Equal(t, int32(http.StatusOK), result.GetStatusCode())
		require.Equal(t, "PHPSESSID=rotated", lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "Cookie"))
		require.NotContains(t, lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "Cookie"), "Path=")
	})

	t.Run("cross origin strips credentials", func(t *testing.T) {
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" || request.Header.Get("X-Api-Key") != "" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.WriteHeader(http.StatusOK)
		})

		location := fmt.Sprintf("http://%s/target", utils.HostPort(host, port))
		result, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			Request:  "GET /start HTTP/1.1\r\nHost: source.invalid\r\nCookie: session=secret\r\nAuthorization: Bearer secret\r\nX-Api-Key: secret\r\n\r\n",
			Response: "HTTP/1.1 302 Found\r\nLocation: " + location + "\r\n\r\n",
		})
		require.NoError(t, err)
		require.Equal(t, int32(http.StatusOK), result.GetStatusCode())
		require.Empty(t, lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "Cookie"))
		require.Empty(t, lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "Authorization"))
		require.Empty(t, lowhttp.GetHTTPPacketHeader(result.GetRequestRaw(), "X-Api-Key"))
	})

	t.Run("307 preserves method and body", func(t *testing.T) {
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPost || string(body) != "a=b" || !strings.HasPrefix(request.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.WriteHeader(http.StatusOK)
		})

		target := utils.HostPort(host, port)
		result, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			Request:  "POST /submit HTTP/1.1\r\nHost: " + target + "\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 3\r\n\r\na=b",
			Response: "HTTP/1.1 307 Temporary Redirect\r\nLocation: result\r\n\r\n",
		})
		require.NoError(t, err)
		require.Equal(t, int32(http.StatusOK), result.GetStatusCode())
		require.Equal(t, http.MethodPost, lowhttp.GetHTTPRequestMethod(result.GetRequestRaw()))
		require.Equal(t, "a=b", string(lowhttp.GetHTTPPacketBody(result.GetRequestRaw())))
	})

	t.Run("302 preserves non-POST method and body", func(t *testing.T) {
		host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPatch || string(body) != `{"enabled":true}` {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.WriteHeader(http.StatusOK)
		})

		target := utils.HostPort(host, port)
		result, err := client.RedirectRequest(context.Background(), &ypb.RedirectRequestParams{
			Request:  "PATCH /resource HTTP/1.1\r\nHost: " + target + "\r\nContent-Type: application/json\r\nContent-Length: 16\r\n\r\n{\"enabled\":true}",
			Response: "HTTP/1.1 302 Found\r\nLocation: updated\r\n\r\n",
		})
		require.NoError(t, err)
		require.Equal(t, int32(http.StatusOK), result.GetStatusCode())
		require.Equal(t, http.MethodPatch, result.GetMethod())
		require.Equal(t, http.MethodPatch, lowhttp.GetHTTPRequestMethod(result.GetRequestRaw()))
		require.Equal(t, `{"enabled":true}`, string(lowhttp.GetHTTPPacketBody(result.GetRequestRaw())))
	})
}

func TestSplitFuzzerRedirectFlows(t *testing.T) {
	original := &lowhttp.LowhttpResponse{RawRequest: []byte("original")}
	intermediate := &lowhttp.LowhttpResponse{RawRequest: []byte("intermediate")}
	final := &lowhttp.LowhttpResponse{RawRequest: []byte("final")}
	tests := []struct {
		name        string
		flows       []*lowhttp.RedirectFlow
		final       *lowhttp.LowhttpResponse
		wantRecords []*lowhttp.LowhttpResponse
		wantRequest string
	}{
		{name: "no flows"},
		{name: "original only", flows: []*lowhttp.RedirectFlow{{RespRecord: original, Request: []byte("original")}}, final: original},
		{
			name:        "multiple hops",
			flows:       []*lowhttp.RedirectFlow{{RespRecord: original}, {RespRecord: intermediate}, {RespRecord: final, Request: []byte("final")}},
			final:       final,
			wantRecords: []*lowhttp.LowhttpResponse{original, intermediate},
			wantRequest: "final",
		},
		{
			name:        "missing intermediate flow and response",
			flows:       []*lowhttp.RedirectFlow{nil, {}, {RespRecord: intermediate}, {RespRecord: final, Request: []byte("final")}},
			final:       final,
			wantRecords: []*lowhttp.LowhttpResponse{intermediate},
			wantRequest: "final",
		},
		{
			name:  "missing final flow does not repeat original",
			flows: []*lowhttp.RedirectFlow{{RespRecord: original}, nil},
			final: original,
		},
		{
			name:  "final response absent from history",
			flows: []*lowhttp.RedirectFlow{{RespRecord: original}, {RespRecord: intermediate}},
			final: final,
		},
		{
			name:        "trailing nil after final",
			flows:       []*lowhttp.RedirectFlow{{RespRecord: original}, {RespRecord: final, Request: []byte("final")}, nil},
			final:       final,
			wantRecords: []*lowhttp.LowhttpResponse{original},
			wantRequest: "final",
		},
		{
			name:        "empty final request",
			flows:       []*lowhttp.RedirectFlow{{RespRecord: original}, {RespRecord: final}},
			final:       final,
			wantRecords: []*lowhttp.LowhttpResponse{original},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, request := splitFuzzerRedirectFlows(tt.flows, tt.final)
			require.Len(t, records, len(tt.wantRecords))
			for i := range records {
				require.Same(t, tt.wantRecords[i], records[i])
			}
			require.Equal(t, tt.wantRequest, string(request))
		})
	}
}

func TestHTTPFuzzerRedirectResponsesAreOrderedAndUnique(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			writer.Header().Set("Location", "/middle")
			writer.WriteHeader(http.StatusMovedPermanently)
		case "/middle":
			writer.Header().Set("Location", "/final")
			writer.WriteHeader(http.StatusFound)
		case "/final":
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	client, err := NewLocalClient()
	require.NoError(t, err)
	tests := []struct {
		name             string
		start            string
		noFollow         bool
		redirectTimes    float64
		wantPaths        []string
		wantStatusCodes  []int32
		wantHistoryCount int
	}{
		{"two hops", "/start", false, 3, []string{"/start", "/middle", "/final"}, []int32{301, 302, 200}, 3},
		{"one hop", "/middle", false, 3, []string{"/middle", "/final"}, []int32{302, 200}, 2},
		{"no redirect", "/final", false, 3, []string{"/final"}, []int32{200}, 1},
		{"redirect limit", "/start", false, 1, []string{"/start", "/middle"}, []int32{301, 302}, 2},
		{"no follow", "/start", true, 3, []string{"/start"}, []int32{301}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
				Request:                  fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", tt.start, utils.HostPort(host, port)),
				NoFollowRedirect:         tt.noFollow,
				RedirectTimes:            tt.redirectTimes,
				PerRequestTimeoutSeconds: 5,
			})
			require.NoError(t, err)

			var responses []*ypb.FuzzerResponse
			for {
				rsp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				responses = append(responses, rsp)
			}
			require.Len(t, responses, len(tt.wantPaths), "each request should be emitted exactly once")
			indexes := make(map[string]struct{}, len(responses))
			for i, rsp := range responses {
				require.True(t, rsp.GetOk(), "response %d: %s", i, rsp.GetReason())
				require.Equal(t, tt.wantStatusCodes[i], rsp.GetStatusCode())
				require.Equal(t, tt.wantPaths[i], lowhttp.GetHTTPRequestPath(rsp.GetRequestRaw()))
				require.Equal(t, tt.wantStatusCodes[i], int32(lowhttp.GetStatusCodeFromResponse(rsp.GetResponseRaw())))
				require.NotEmpty(t, rsp.GetHiddenIndex())
				_, duplicate := indexes[rsp.GetHiddenIndex()]
				require.False(t, duplicate, "each hop should have its own hidden index")
				indexes[rsp.GetHiddenIndex()] = struct{}{}
			}
			flows := responses[len(responses)-1].GetRedirectFlows()
			require.Len(t, flows, tt.wantHistoryCount)
			for i, flow := range flows {
				require.Equal(t, tt.wantPaths[i], lowhttp.GetHTTPRequestPath(flow.GetRequest()))
				require.Equal(t, tt.wantStatusCodes[i], int32(lowhttp.GetStatusCodeFromResponse(flow.GetResponse())))
			}
		})
	}
}

func TestHTTPFuzzerRedirect405RepairIsVisibleInHistory(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/start":
			writer.Header().Set("Location", "/protected")
			writer.WriteHeader(http.StatusFound)
		case "/protected":
			if request.Method == http.MethodGet {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPost || string(body) != "a=b" {
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			writer.Header().Set("Location", "/end")
			writer.WriteHeader(http.StatusFound)
		case "/end":
			writer.WriteHeader(http.StatusOK)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  fmt.Sprintf("POST /start HTTP/1.1\r\nHost: %s\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 3\r\n\r\na=b", utils.HostPort(host, port)),
		RedirectTimes:            2,
		PerRequestTimeoutSeconds: 5,
	})
	require.NoError(t, err)
	var responses []*ypb.FuzzerResponse
	for {
		rsp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		responses = append(responses, rsp)
	}
	require.Len(t, responses, 4)
	wantPaths := []string{"/start", "/protected", "/protected", "/end"}
	wantMethods := []string{http.MethodPost, http.MethodGet, http.MethodPost, http.MethodGet}
	wantCodes := []int32{302, 405, 302, 200}
	for i, rsp := range responses {
		require.Equal(t, wantPaths[i], lowhttp.GetHTTPRequestPath(rsp.GetRequestRaw()))
		require.Equal(t, wantMethods[i], lowhttp.GetHTTPRequestMethod(rsp.GetRequestRaw()))
		require.Equal(t, wantMethods[i], rsp.GetMethod(), "method and displayed request must agree")
		require.Equal(t, wantCodes[i], rsp.GetStatusCode())
	}
	require.Equal(t, fmt.Sprintf("http://%s/end", utils.HostPort(host, port)), responses[3].GetUrl())
	require.Equal(t, utils.HostPort(host, port), responses[3].GetHost())
	flows := responses[3].GetRedirectFlows()
	require.Len(t, flows, 4)
	for i, flow := range flows {
		require.Equal(t, wantPaths[i], lowhttp.GetHTTPRequestPath(flow.GetRequest()))
		require.Equal(t, wantMethods[i], lowhttp.GetHTTPRequestMethod(flow.GetRequest()))
		require.Equal(t, wantCodes[i], int32(lowhttp.GetStatusCodeFromResponse(flow.GetResponse())))
	}
}

func TestHTTPFuzzerRedirectFinalHostMatchesRequest(t *testing.T) {
	finalHost, finalPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	finalTarget := utils.HostPort(finalHost, finalPort)
	startHost, startPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Location", "http://"+finalTarget+"/end")
		writer.WriteHeader(http.StatusFound)
	})
	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:                  fmt.Sprintf("GET /start HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(startHost, startPort)),
		RedirectTimes:            1,
		PerRequestTimeoutSeconds: 5,
	})
	require.NoError(t, err)
	var responses []*ypb.FuzzerResponse
	for {
		rsp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		responses = append(responses, rsp)
	}
	require.Len(t, responses, 2)
	final := responses[1]
	require.Equal(t, int32(http.StatusOK), final.GetStatusCode())
	require.Equal(t, finalTarget, lowhttp.GetHTTPPacketHeader(final.GetRequestRaw(), "Host"))
	require.Equal(t, finalTarget, final.GetHost())
	require.Equal(t, "http://"+finalTarget+"/end", final.GetUrl())
}
