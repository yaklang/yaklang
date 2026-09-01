package scannode

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignS3Request_AWSOfficialGetObjectVector(t *testing.T) {
	// AWS S3's published SigV4 example:
	// https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	require.NoError(t, err)
	req.Header.Set("Range", "bytes=0-9")
	err = signS3Request(req, objectStoreCredentials{
		AccessKey: newSecretValue("AKIAIOSFODNN7EXAMPLE"),
		SecretKey: newSecretValue("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
	}, "us-east-1", emptySHA256Hex, time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t,
		"AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,"+
			"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date,"+
			"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41",
		req.Header.Get("Authorization"),
	)
}

func TestAWSURIEncode_ObjectNames(t *testing.T) {
	require.Equal(t, "/bucket/%E4%B8%AD%E6%96%87%20name/%24file%2B%25.txt", awsURIEncode("/bucket/中文 name/$file+%.txt", false))
	require.Equal(t, "uploadId=a%2Fb%2Bc%3D&x=%E4%B8%AD%E6%96%87%20value", canonicalS3Query([]queryValue{
		{Key: "x", Value: "中文 value"},
		{Key: "uploadId", Value: "a/b+c="},
	}))
	endpoint, err := url.Parse("https://objects.example.test/base")
	require.NoError(t, err)
	client := &s3ObjectStoreClient{endpoint: endpoint, virtualHostStyle: true}
	objectURL := client.objectURL("test-bucket", "中文 name/$file+%.txt", nil)
	require.Equal(t, "test-bucket.objects.example.test", objectURL.Host)
	require.Equal(t, "/base/%E4%B8%AD%E6%96%87%20name/%24file%2B%25.txt", objectURL.EscapedPath())
}

func TestSecretValueRedactsFormattingAndJSON(t *testing.T) {
	const accessKey = "ACCESS_SHOULD_NOT_LEAK"
	const secretKey = "SECRET_SHOULD_NOT_LEAK"
	const token = "TOKEN_SHOULD_NOT_LEAK"
	cfg := &SSAArtifactUploadConfig{
		STSAccessKey: newSecretValue(accessKey), STSSecretKey: newSecretValue(secretKey), STSSessionToken: newSecretValue(token),
	}
	formatted := fmt.Sprintf("%+v %#v %s", cfg, cfg, cfg.STSSecretKey)
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	for _, value := range []string{accessKey, secretKey, token} {
		require.NotContains(t, formatted, value)
		require.NotContains(t, string(encoded), value)
	}
	require.Contains(t, formatted, "[REDACTED]")
}

func TestValidateSSAUploadConfig_RequiresExplicitHTTPAndBoundsPartSize(t *testing.T) {
	t.Setenv("SCANNODE_SSA_ALLOW_HTTP", "")
	cfg := testSSAUploadConfig("http://127.0.0.1:9000")
	cfg.AllowHTTP = false
	_, err := validateSSAUploadConfig(cfg)
	require.ErrorContains(t, err, "explicit development allowance")

	cfg.AllowHTTP = true
	_, err = validateSSAUploadConfig(cfg)
	require.NoError(t, err)

	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "9999")
	require.EqualValues(t, maxSSAMultipartPartSizeBytes, readSSAMultipartPartSize())
	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "1")
	require.EqualValues(t, minSSAMultipartPartSizeBytes, readSSAMultipartPartSize())
	t.Setenv("SCANNODE_SSA_MULTIPART_CONCURRENCY", "99")
	require.Equal(t, maxSSAMultipartConcurrency, readSSAMultipartConcurrency())
	t.Setenv("SCANNODE_SSA_MULTIPART_CONCURRENCY", "0")
	require.Equal(t, 1, readSSAMultipartConcurrency())
}

func TestSSATLS_DefaultSkipVerifyAndExplicitPrivateCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	t.Setenv("SCANNODE_SSA_TICKET_TLS_VERIFY", "")
	t.Setenv("SCANNODE_SSA_TICKET_TLS_CA_FILE", "")
	client, err := newSSATicketHTTPClient()
	require.NoError(t, err)
	resp, err := client.Get(server.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	client.CloseIdleConnections()

	t.Setenv("SCANNODE_SSA_TICKET_TLS_VERIFY", "true")
	client, err = newSSATicketHTTPClient()
	require.NoError(t, err)
	_, err = client.Get(server.URL)
	require.Error(t, err)
	client.CloseIdleConnections()

	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.TLS.Certificates[0].Certificate[0]})
	caPath := t.TempDir() + "/private-ca.pem"
	require.NoError(t, os.WriteFile(caPath, certificatePEM, 0o600))
	t.Setenv("SCANNODE_SSA_TICKET_TLS_CA_FILE", caPath)
	client, err = newSSATicketHTTPClient()
	require.NoError(t, err)
	resp, err = client.Get(server.URL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	client.CloseIdleConnections()

	// Object-store TLS follows the same compatibility default requested for
	// private ScanNode deployments, while still allowing strict verification.
	t.Setenv("SCANNODE_SSA_TLS_CA_FILE", "")
	cfg := testSSAUploadConfig(server.URL)
	store, err := newS3ObjectStoreClient(cfg)
	require.NoError(t, err)
	_, err = store.Put(context.Background(), PutRequest{
		Bucket: cfg.Bucket, ObjectKey: cfg.ObjectKey, Body: bytes.NewReader([]byte("tls")), Size: 3, PayloadSHA256: sha256Hex([]byte("tls")),
	})
	require.NoError(t, err)
	store.Close()

	cfg.TLSVerify = true
	store, err = newS3ObjectStoreClient(cfg)
	require.NoError(t, err)
	_, err = store.Put(context.Background(), PutRequest{
		Bucket: cfg.Bucket, ObjectKey: cfg.ObjectKey, Body: bytes.NewReader([]byte("tls")), Size: 3, PayloadSHA256: sha256Hex([]byte("tls")),
	})
	require.Error(t, err)
	store.Close()

	cfg.TLSCAFile = caPath
	store, err = newS3ObjectStoreClient(cfg)
	require.NoError(t, err)
	_, err = store.Put(context.Background(), PutRequest{
		Bucket: cfg.Bucket, ObjectKey: cfg.ObjectKey, Body: bytes.NewReader([]byte("tls")), Size: 3, PayloadSHA256: sha256Hex([]byte("tls")),
	})
	require.NoError(t, err)
	store.Close()
}

func TestS3ObjectStore_PutMultipartRetryAndEncoding(t *testing.T) {
	var mu sync.Mutex
	requests := make([]string, 0, 8)
	parts := make(map[int][]byte)
	var retryCount atomic.Int32
	var completeAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		mu.Unlock()
		require.Contains(t, r.Header.Get("Authorization"), "Credential=test-access/")
		require.Equal(t, "test-token", r.Header.Get("x-amz-security-token"))
		require.NotEmpty(t, r.Header.Get("x-amz-content-sha256"))

		switch {
		case r.Method == http.MethodPut && r.URL.Query().Get("uploadId") == "upload-1":
			partNumber := r.URL.Query().Get("partNumber")
			if partNumber == "1" && retryCount.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `<Error><Code>SlowDown</Code><Message>retry later</Message></Error>`)
				return
			}
			var number int
			_, _ = fmt.Sscanf(partNumber, "%d", &number)
			parts[number] = append([]byte(nil), body...)
			w.Header().Set("ETag", fmt.Sprintf(`"part-%d"`, number))
			w.Header().Set("x-amz-request-id", fmt.Sprintf("part-request-%d", number))
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && hasRawQueryKey(r.URL.RawQuery, "uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") == "upload-1":
			if completeAttempts.Add(1) == 1 {
				// S3 may report completion errors inside a 200 response after the
				// whitespace keepalive. This must remain a failure/retry path.
				_, _ = io.WriteString(w, `<?xml version="1.0"?><Error><Code>SlowDown</Code><Message>retry completion</Message></Error>`)
				return
			}
			var complete struct {
				Parts []struct {
					PartNumber int    `xml:"PartNumber"`
					ETag       string `xml:"ETag"`
				} `xml:"Part"`
			}
			require.NoError(t, xml.Unmarshal(body, &complete))
			require.Len(t, complete.Parts, 2)
			w.Header().Set("x-amz-request-id", "complete-request")
			_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><ETag>"complete-etag"</ETag></CompleteMultipartUploadResult>`)
		case r.Method == http.MethodPut:
			require.Equal(t, []byte("small payload"), body)
			w.Header().Set("ETag", `"small-etag"`)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := testSSAUploadConfig(server.URL)
	provider := func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }
	session, err := newSSAObjectStoreUploadSession(provider, nil)
	require.NoError(t, err)
	defer session.Close()

	smallStats, err := session.uploadBytes(context.Background(), []byte("small payload"), "path/中文 name+$%.txt")
	require.NoError(t, err)
	require.Equal(t, "small-etag", smallStats.ETag)

	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "5")
	payload := bytes.Repeat([]byte("x"), 5*1024*1024+17)
	largeStats, err := session.upload(context.Background(), bytes.NewReader(payload), int64(len(payload)), "large/object.bin")
	require.NoError(t, err)
	require.EqualValues(t, len(payload), largeStats.Bytes)
	require.Equal(t, 2, largeStats.Parts)
	require.Equal(t, "complete-request", largeStats.RequestID)
	require.Equal(t, payload, append(parts[1], parts[2]...))
	require.EqualValues(t, 2, retryCount.Load())
	require.EqualValues(t, 2, completeAttempts.Load())

	mu.Lock()
	joined := strings.Join(requests, "\n")
	mu.Unlock()
	require.Contains(t, joined, "/test-bucket/path/%E4%B8%AD%E6%96%87%20name%2B%24%25.txt")
}

func TestObjectStoreSession_RefreshesOnlyExpiredCredentials(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		wantRefreshes int32
		wantError     bool
	}{
		{name: "expired", code: "ExpiredToken", wantRefreshes: 1},
		{name: "denied", code: "AccessDenied", wantRefreshes: 0, wantError: true},
		{name: "bad signature", code: "SignatureDoesNotMatch", wantRefreshes: 0, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if tc.code != "" && call == 1 {
					w.WriteHeader(http.StatusForbidden)
					_, _ = io.WriteString(w, `<Error><Code>`+tc.code+`</Code><Message>rejected test-access test-secret test-token</Message></Error>`)
					return
				}
				if tc.code == "ExpiredToken" {
					require.Contains(t, r.Header.Get("Authorization"), "Credential=fresh-access/")
				}
				w.Header().Set("ETag", `"ok"`)
			}))
			defer server.Close()

			cfg := testSSAUploadConfig(server.URL)
			var refreshes atomic.Int32
			provider := func(force bool) (*SSAArtifactUploadConfig, error) {
				cp := *cfg
				if force {
					refreshes.Add(1)
					cp.STSAccessKey = newSecretValue("fresh-access")
				}
				return &cp, nil
			}
			session, err := newSSAObjectStoreUploadSession(provider, nil)
			require.NoError(t, err)
			defer session.Close()
			_, err = session.uploadBytes(context.Background(), []byte("payload"), "object.bin")
			if tc.wantError {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "test-access")
				require.NotContains(t, err.Error(), "test-secret")
				require.NotContains(t, err.Error(), "test-token")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantRefreshes, refreshes.Load())
		})
	}
}

func TestObjectStoreSession_CancelAbortsMultipart(t *testing.T) {
	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "5")
	partStarted := make(chan struct{})
	aborted := make(chan struct{})
	var partOnce, abortOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && hasRawQueryKey(r.URL.RawQuery, "uploads"):
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>cancel-upload</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && r.URL.Query().Get("uploadId") == "cancel-upload":
			_, _ = io.Copy(io.Discard, r.Body)
			partOnce.Do(func() { close(partStarted) })
			<-r.Context().Done()
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") == "cancel-upload":
			abortOnce.Do(func() { close(aborted) })
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := testSSAUploadConfig(server.URL)
	session, err := newSSAObjectStoreUploadSession(func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }, nil)
	require.NoError(t, err)
	defer session.Close()
	ctx, cancel := context.WithCancel(context.Background())
	pipeReader, pipeWriter := io.Pipe()
	defer pipeWriter.Close()
	errCh := make(chan error, 1)
	go func() {
		_, uploadErr := session.upload(ctx, pipeReader, -1, "cancel.bin")
		errCh <- uploadErr
	}()
	go func() {
		_, _ = pipeWriter.Write(bytes.Repeat([]byte("x"), 5*1024*1024))
	}()
	select {
	case <-partStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("multipart part did not start")
	}
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
	select {
	case <-aborted:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled multipart upload was not aborted")
	}
}

func TestObjectStoreSession_CompleteFailureAbortsMultipart(t *testing.T) {
	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "5")
	aborted := make(chan struct{})
	var abortOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && hasRawQueryKey(r.URL.RawQuery, "uploads"):
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>complete-failure</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && r.URL.Query().Get("uploadId") == "complete-failure":
			w.Header().Set("ETag", `"part"`)
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") == "complete-failure":
			_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>completion denied</Message></Error>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") == "complete-failure":
			abortOnce.Do(func() { close(aborted) })
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cfg := testSSAUploadConfig(server.URL)
	session, err := newSSAObjectStoreUploadSession(func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }, nil)
	require.NoError(t, err)
	defer session.Close()
	_, err = session.uploadBytes(context.Background(), bytes.Repeat([]byte("x"), 5*1024*1024+1), "complete-failure.bin")
	require.Error(t, err)
	require.Equal(t, "permission_denied", classifyObjectStoreError(err))
	select {
	case <-aborted:
	case <-time.After(5 * time.Second):
		t.Fatal("failed completion was not aborted")
	}
}

func TestClassifyObjectStoreErrorStructured(t *testing.T) {
	require.Equal(t, "sts_expired", classifyObjectStoreError(&ObjectStoreError{Operation: "put", Code: "ExpiredToken"}))
	require.Equal(t, "permission_denied", classifyObjectStoreError(&ObjectStoreError{Operation: "put", Code: "AccessDenied"}))
	require.Equal(t, "signature_invalid", classifyObjectStoreError(&ObjectStoreError{Operation: "put", Code: "SignatureDoesNotMatch"}))
	require.Equal(t, "upload_canceled", classifyObjectStoreError(context.Canceled))
}

func testSSAUploadConfig(endpoint string) *SSAArtifactUploadConfig {
	return &SSAArtifactUploadConfig{
		Endpoint: endpoint, Bucket: "test-bucket", ObjectKey: "object.bin", Region: "us-east-1",
		UseSSL: strings.HasPrefix(endpoint, "https://"), AllowHTTP: strings.HasPrefix(endpoint, "http://"),
		STSAccessKey: newSecretValue("test-access"), STSSecretKey: newSecretValue("test-secret"), STSSessionToken: newSecretValue("test-token"),
	}
}

func hasRawQueryKey(rawQuery, key string) bool {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false
	}
	_, ok := values[key]
	return ok
}
