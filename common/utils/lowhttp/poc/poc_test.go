package poc

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestPocWithRandomJA3(t *testing.T) {
	token := utils.RandStringBytes(128)
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: %d\r\n\r\n%s", len(token), token)))

	for i := 0; i < 16; i++ {
		rsp, _, err := DoGET("http://"+utils.HostPort(host, port), WithRandomJA3(true))
		require.NoError(t, err)
		require.Containsf(t, string(rsp.RawPacket), token, "invalid response")
	}
}

func TestPocRequestWithSession(t *testing.T) {
	token, token2, token3 := utils.RandStringBytes(10), utils.RandStringBytes(10), utils.RandStringBytes(10)
	cookieStr := fmt.Sprintf("%s=%s", token, token2)

	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nConnection: close\r\nSet-Cookie: %s\r\n\r\n", cookieStr)))

	// get cookie from server
	_, _, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)

	// test HTTP / DO
	// if request has cookie
	_, req, err := HTTP(fmt.Sprintf(`GET / HTTP/1.1
Host: %s
`, utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	require.Contains(t, string(req), cookieStr)

	_, req2, err := Do(http.MethodGet, fmt.Sprintf("http://%s", utils.HostPort(host, port)), WithSession(token3))
	require.NoError(t, err)
	cookie, err := req2.Cookie(token)
	require.NoError(t, err)
	require.Equal(t, token2, cookie.Value)
}

func TestWithPostParams(t *testing.T) {
	tests := []struct {
		name                string
		input               any
		expectedContentType string
		expectedParams      map[string]string
		description         string
	}{
		{
			name:                "map_input",
			input:               map[string]string{"username": "admin", "password": "123456", "token": "abc123"},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      map[string]string{"username": "admin", "password": "123456", "token": "abc123"},
			description:         "map input should be converted to form data with correct content type",
		},
		{
			name:                "empty_value_map",
			input:               map[string]string{"username": ""},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      map[string]string{"username": ""},
			description:         "empty value map should set content type and empty parameter",
		},
		{
			name: "nested_map_input",
			input: map[string]interface{}{
				"user": map[string]string{
					"name": "admin",
					"role": "user",
				},
				"settings": []string{"option1", "option2"},
				"simple":   "value",
			},
			expectedContentType: "application/x-www-form-urlencoded",
			expectedParams:      nil, // 后续代码单独进行测试
			description:         "嵌套map测试",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				contentType := request.Header.Get("Content-Type")
				body := ""
				if request.Body != nil {
					bodyBytes, _ := io.ReadAll(request.Body)
					body = string(bodyBytes)
				}

				response := fmt.Sprintf("Content-Type: %s\nBody: %s", contentType, body)
				writer.WriteHeader(200)
				writer.Write([]byte(response))
			})

			requestURL := fmt.Sprintf("http://%s", utils.HostPort(host, port))

			rsp, req, err := DoPOST(requestURL, WithPostParams(tt.input))
			require.NoError(t, err, tt.description)
			require.NotNil(t, rsp, "Response should not be nil")
			require.NotNil(t, req, "Request should not be nil")

			t.Logf("raw packet:%s", rsp.RawRequest)

			if tt.expectedContentType != "" {
				require.Equal(t, tt.expectedContentType, req.Header.Get("Content-Type"))
			}

			responseBody := string(rsp.RawPacket)
			bodyStart := strings.Index(responseBody, "Body: ")
			require.Greater(t, bodyStart, -1, "Should find body in response")
			formData := responseBody[bodyStart+6:] // Skip "Body: "

			// Parse the form data to check individual parameters
			values, err := url.ParseQuery(formData)
			require.NoError(t, err, "Should be able to parse form data")

			if tt.expectedParams != nil {
				// Check each expected parameter individually
				for expectedKey, expectedValue := range tt.expectedParams {
					actualValue := values.Get(expectedKey)
					require.Equal(t, expectedValue, actualValue,
						"Parameter %s should have value %s, got %s", expectedKey, expectedValue, actualValue)
				}

				// Verify we have the expected number of parameters
				require.Equal(t, len(tt.expectedParams), len(values),
					"Should have exactly %d parameters", len(tt.expectedParams))
			}

			// 嵌套map测试
			if tt.name == "nested_map_input" {
				t.Logf("Nested map form data: %s", formData)

				// Verify basic parameters
				require.Equal(t, "value", values.Get("simple"), "Simple parameter should be preserved")

				// Verify nested structures are present as form parameters
				require.NotEmpty(t, values.Get("user"), "User parameter should not be empty")
				require.NotEmpty(t, values.Get("settings"), "Settings parameter should not be empty")

				// Check that complex values are properly encoded
				userValue := values.Get("user")
				require.Contains(t, userValue, "admin", "User value should contain admin")
				require.Contains(t, userValue, "user", "User value should contain role")

				settingsValue := values.Get("settings")
				require.Contains(t, settingsValue, "option1", "Settings should contain option1")
				require.Contains(t, settingsValue, "option2", "Settings should contain option2")

				t.Logf("✓ Nested map parameters:")
				t.Logf("  - simple: %s", values.Get("simple"))
				t.Logf("  - user: %s", values.Get("user"))
				t.Logf("  - settings: %s", values.Get("settings"))
			}

			t.Logf("✓ %s: %s", tt.name, tt.description)
		})
	}
}
