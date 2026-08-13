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
