package lowhttp

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedirectMethodRepairSwitchesMethodWithoutConsumingRedirect(t *testing.T) {
	tests := []struct {
		name            string
		firstStatus     int
		rejectedMethod  string
		rejectionStatus int
		repaired        string
		protectedBody   string
		wantMethods     []string
		wantStatusCodes []int
	}{
		{
			name:        "400 after 302 POST to GET, retry with POST body",
			firstStatus: http.StatusFound, rejectedMethod: http.MethodGet, rejectionStatus: http.StatusBadRequest, repaired: http.MethodPost,
			protectedBody:   "a=b",
			wantMethods:     []string{http.MethodPost, http.MethodGet, http.MethodPost, http.MethodGet},
			wantStatusCodes: []int{302, 400, 302, 200},
		},
		{
			name:        "405 after 302 POST to GET, retry with POST body",
			firstStatus: http.StatusFound, rejectedMethod: http.MethodGet, rejectionStatus: http.StatusMethodNotAllowed, repaired: http.MethodPost,
			protectedBody:   "a=b",
			wantMethods:     []string{http.MethodPost, http.MethodGet, http.MethodPost, http.MethodGet},
			wantStatusCodes: []int{302, 405, 302, 200},
		},
		{
			name:        "400 after 307 inherited POST, retry with GET",
			firstStatus: http.StatusTemporaryRedirect, rejectedMethod: http.MethodPost, rejectionStatus: http.StatusBadRequest, repaired: http.MethodGet,
			wantMethods:     []string{http.MethodPost, http.MethodPost, http.MethodGet, http.MethodGet},
			wantStatusCodes: []int{307, 400, 302, 200},
		},
		{
			name:        "405 after 307 inherited POST, retry with GET",
			firstStatus: http.StatusTemporaryRedirect, rejectedMethod: http.MethodPost, rejectionStatus: http.StatusMethodNotAllowed, repaired: http.MethodGet,
			wantMethods:     []string{http.MethodPost, http.MethodPost, http.MethodGet, http.MethodGet},
			wantStatusCodes: []int{307, 405, 302, 200},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/start":
					w.Header().Set("Location", "/protected")
					w.WriteHeader(tt.firstStatus)
				case "/protected":
					body, _ := io.ReadAll(r.Body)
					if r.Method == tt.rejectedMethod {
						w.WriteHeader(tt.rejectionStatus)
						return
					}
					if r.Method != tt.repaired || string(body) != tt.protectedBody {
						w.WriteHeader(http.StatusTeapot)
						return
					}
					w.Header().Set("Location", "/end")
					w.WriteHeader(http.StatusFound)
				case "/end":
					if r.Method != http.MethodGet {
						w.WriteHeader(http.StatusTeapot)
						return
					}
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			host := strings.TrimPrefix(server.URL, "http://")
			request := fmt.Sprintf("POST /start HTTP/1.1\r\nHost: %s\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 3\r\n\r\na=b", host)
			response, err := HTTP(WithRequest(request), WithRedirectTimes(2), WithTimeoutFloat(3))
			require.NoError(t, err)
			require.NotNil(t, response)
			require.Equal(t, http.StatusOK, GetStatusCodeFromResponse(response.RawPacket), "the repair must not use up the second redirect")
			require.Len(t, response.RedirectRawPackets, 4, "the original, rejected, repaired and final response must all be recorded")
			wantPaths := []string{"/start", "/protected", "/protected", "/end"}
			for i, flow := range response.RedirectRawPackets {
				require.NotNil(t, flow)
				require.NotNil(t, flow.RespRecord)
				require.Equal(t, wantPaths[i], GetHTTPRequestPath(flow.Request))
				require.Equal(t, tt.wantMethods[i], GetHTTPRequestMethod(flow.Request))
				require.Equal(t, tt.wantStatusCodes[i], GetStatusCodeFromResponse(flow.Response))
				require.NotNil(t, flow.RespRecord.RequestInstance)
				require.Equal(t, tt.wantMethods[i], flow.RespRecord.RequestInstance.Method)
			}
			require.Equal(t, tt.protectedBody, string(GetHTTPPacketBody(response.RedirectRawPackets[2].Request)))
			require.Same(t, response, response.RedirectRawPackets[3].RespRecord)
		})
	}
}

func TestRedirectMethodRepairOnlyOnceAcross400And405(t *testing.T) {
	var secondRepairAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Set("Location", "/first")
			w.WriteHeader(http.StatusFound)
		case "/first":
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Location", "/second")
			w.WriteHeader(http.StatusFound)
		case "/second":
			if r.Method == http.MethodPost {
				secondRepairAttempts.Add(1)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	request := fmt.Sprintf("POST /start HTTP/1.1\r\nHost: %s\r\nContent-Length: 3\r\n\r\na=b", strings.TrimPrefix(server.URL, "http://"))
	response, err := HTTP(WithRequest(request), WithRedirectTimes(3), WithTimeoutFloat(3))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusMethodNotAllowed, GetStatusCodeFromResponse(response.RawPacket))
	require.Zero(t, secondRepairAttempts.Load())
	require.Len(t, response.RedirectRawPackets, 4)
	require.Equal(t, []string{"/start", "/first", "/first", "/second"}, []string{
		GetHTTPRequestPath(response.RedirectRawPackets[0].Request),
		GetHTTPRequestPath(response.RedirectRawPackets[1].Request),
		GetHTTPRequestPath(response.RedirectRawPackets[2].Request),
		GetHTTPRequestPath(response.RedirectRawPackets[3].Request),
	})
}

func TestRedirectMethodRepairDoesNotRetryUnchangedGET(t *testing.T) {
	var rejectedAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "/protected")
			w.WriteHeader(http.StatusFound)
			return
		}
		rejectedAttempts.Add(1)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(server.Close)

	request := fmt.Sprintf("GET /start HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(server.URL, "http://"))
	response, err := HTTP(WithRequest(request), WithRedirectTimes(3), WithTimeoutFloat(3))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusMethodNotAllowed, GetStatusCodeFromResponse(response.RawPacket))
	require.EqualValues(t, 1, rejectedAttempts.Load())
	require.Len(t, response.RedirectRawPackets, 2)
}

func TestRedirectMethodRepairDoesNotRetry401(t *testing.T) {
	var repairAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "/protected")
			w.WriteHeader(http.StatusFound)
			return
		}
		if r.Method == http.MethodPost {
			repairAttempts.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	request := fmt.Sprintf("POST /start HTTP/1.1\r\nHost: %s\r\nContent-Length: 3\r\n\r\na=b", strings.TrimPrefix(server.URL, "http://"))
	response, err := HTTP(WithRequest(request), WithRedirectTimes(3), WithTimeoutFloat(3))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusUnauthorized, GetStatusCodeFromResponse(response.RawPacket))
	require.Zero(t, repairAttempts.Load())
	require.Len(t, response.RedirectRawPackets, 2)
}

func TestRedirectMethodRepairDoesNotRestoreCrossOriginAuthorization(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusTeapot)
			return
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+"/protected")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(source.Close)

	request := fmt.Sprintf("POST /start HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer secret\r\nContent-Length: 3\r\n\r\na=b", strings.TrimPrefix(source.URL, "http://"))
	response, err := HTTP(WithRequest(request), WithRedirectTimes(1), WithTimeoutFloat(3))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, http.StatusOK, GetStatusCodeFromResponse(response.RawPacket))
	require.Len(t, response.RedirectRawPackets, 3)
	for _, flow := range response.RedirectRawPackets[1:] {
		require.Empty(t, GetHTTPPacketHeader(flow.Request, "Authorization"))
	}
}
