package scannode

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestS3ObjectStoreIntegration exercises the same client against a real
// MinIO, AWS S3, or compatible endpoint. It is opt-in so ordinary unit tests
// never need external credentials or mutate a remote bucket.
func TestS3ObjectStoreIntegration(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_SECRET_KEY"))
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("set SCANNODE_S3_INTEGRATION_ENDPOINT and integration credentials")
	}
	bucket := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_BUCKET"))
	createBucket := bucket == ""
	if bucket == "" {
		bucket = fmt.Sprintf("scannode-s3-test-%d", time.Now().UnixNano())
	}
	cfg := &SSAArtifactUploadConfig{
		Endpoint:         endpoint,
		Bucket:           bucket,
		ObjectKey:        "integration/probe.bin",
		Region:           normalizedSSARegion(os.Getenv("SCANNODE_S3_INTEGRATION_REGION")),
		UseSSL:           strings.HasPrefix(endpoint, "https://"),
		AllowHTTP:        strings.HasPrefix(endpoint, "http://"),
		TLSVerify:        strings.EqualFold(os.Getenv("SCANNODE_S3_INTEGRATION_TLS_VERIFY"), "true"),
		VirtualHostStyle: strings.EqualFold(os.Getenv("SCANNODE_S3_INTEGRATION_VIRTUAL_HOST"), "true"),
		STSAccessKey:     accessKey,
		STSSecretKey:     secretKey,
		STSSessionToken:  os.Getenv("SCANNODE_S3_INTEGRATION_SESSION_TOKEN"),
	}
	client, err := newS3ObjectStoreClient(cfg)
	require.NoError(t, err)
	defer client.Close()
	if createBucket {
		require.NoError(t, createS3IntegrationBucket(context.Background(), client, bucket))
	}

	provider := func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }
	session, err := newSSAObjectStoreUploadSession(provider, nil)
	require.NoError(t, err)
	defer session.Close()

	t.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "16")
	tests := []struct {
		name    string
		key     string
		payload []byte
	}{
		{name: "small special key", key: "integration/中文 name + %.txt", payload: []byte("small payload")},
		{name: "five MiB boundary", key: "integration/five-mib.bin", payload: bytes.Repeat([]byte("5"), 5*1024*1024)},
		{name: "multipart", key: "integration/multipart.bin", payload: bytes.Repeat([]byte("m"), 16*1024*1024+257)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			stats, err := session.uploadBytes(ctx, tc.payload, tc.key)
			require.NoError(t, err)
			require.EqualValues(t, len(tc.payload), stats.Bytes)
			downloaded, err := getS3IntegrationObject(ctx, client, bucket, tc.key)
			require.NoError(t, err)
			require.Equal(t, tc.payload, downloaded)
		})
	}

	uploadID, _, err := client.CreateMultipart(context.Background(), CreateRequest{Bucket: bucket, ObjectKey: "integration/abort.bin", ContentType: "application/octet-stream"})
	require.NoError(t, err)
	require.NoError(t, client.AbortMultipart(context.Background(), AbortRequest{Bucket: bucket, ObjectKey: "integration/abort.bin", UploadID: uploadID}))
	remainingUploadIDs, err := listS3IntegrationMultipartUploads(context.Background(), client, bucket)
	require.NoError(t, err)
	require.NotContains(t, remainingUploadIDs, uploadID)
}

func BenchmarkS3ObjectStoreIntegration64MiB(b *testing.B) {
	endpoint := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("SCANNODE_S3_INTEGRATION_SECRET_KEY"))
	if endpoint == "" || accessKey == "" || secretKey == "" {
		b.Skip("set SCANNODE S3 integration endpoint and credentials")
	}
	cfg := &SSAArtifactUploadConfig{
		Endpoint: endpoint, Bucket: fmt.Sprintf("scannode-s3-bench-%d", time.Now().UnixNano()), ObjectKey: "benchmark/payload.bin",
		Region: normalizedSSARegion(os.Getenv("SCANNODE_S3_INTEGRATION_REGION")), UseSSL: strings.HasPrefix(endpoint, "https://"), AllowHTTP: strings.HasPrefix(endpoint, "http://"),
		STSAccessKey: accessKey, STSSecretKey: secretKey, STSSessionToken: os.Getenv("SCANNODE_S3_INTEGRATION_SESSION_TOKEN"),
	}
	client, err := newS3ObjectStoreClient(cfg)
	require.NoError(b, err)
	defer client.Close()
	require.NoError(b, createS3IntegrationBucket(context.Background(), client, cfg.Bucket))
	session, err := newSSAObjectStoreUploadSession(func(bool) (*SSAArtifactUploadConfig, error) { cp := *cfg; return &cp, nil }, nil)
	require.NoError(b, err)
	defer session.Close()
	b.Setenv("SCANNODE_SSA_MULTIPART_PART_SIZE_MB", "16")
	payload := bytes.Repeat([]byte("b"), 64*1024*1024)
	file, err := os.CreateTemp("", "s3-client-benchmark-*.bin")
	require.NoError(b, err)
	path := file.Name()
	defer os.Remove(path)
	_, err = file.Write(payload)
	require.NoError(b, err)
	require.NoError(b, file.Close())
	payload = nil
	b.SetBytes(64 * 1024 * 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		cfg.ObjectKey = fmt.Sprintf("benchmark/payload-%06d.bin", index)
		_, err := session.uploadFile(context.Background(), path, 64*1024*1024, cfg.ObjectKey)
		require.NoError(b, err)
	}
}

func createS3IntegrationBucket(ctx context.Context, client *s3ObjectStoreClient, bucket string) error {
	requestURL := *client.endpoint
	requestURL.Path = strings.TrimSuffix(requestURL.Path, "/") + "/" + bucket
	requestURL.RawPath = awsURIEncode(requestURL.Path, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL.String(), nil)
	if err != nil {
		return err
	}
	if err := signS3Request(req, client.currentCredentials(), client.region, emptySHA256Hex, client.now()); err != nil {
		return err
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSSAObjectStoreErrorBodyBytes))
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusConflict {
		return nil
	}
	return parseS3Error("create_bucket", resp.StatusCode, resp.Header, body)
}

func getS3IntegrationObject(ctx context.Context, client *s3ObjectStoreClient, bucket, objectKey string) ([]byte, error) {
	requestURL := client.objectURL(bucket, objectKey, nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := signS3Request(req, client.currentCredentials(), client.region, emptySHA256Hex, client.now()); err != nil {
		return nil, err
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSSAObjectStoreErrorBodyBytes))
		return nil, parseS3Error("get", resp.StatusCode, resp.Header, body)
	}
	return io.ReadAll(resp.Body)
}

func listS3IntegrationMultipartUploads(ctx context.Context, client *s3ObjectStoreClient, bucket string) ([]string, error) {
	requestURL := *client.endpoint
	basePath := strings.TrimSuffix(requestURL.Path, "/")
	if client.virtualHostStyle {
		requestURL.Host = bucket + "." + requestURL.Host
		requestURL.Path = basePath + "/"
	} else {
		requestURL.Path = basePath + "/" + bucket
	}
	requestURL.RawPath = awsURIEncode(requestURL.Path, false)
	requestURL.RawQuery = canonicalS3Query([]queryValue{{Key: "uploads"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := signS3Request(req, client.currentCredentials(), client.region, emptySHA256Hex, client.now()); err != nil {
		return nil, err
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSSAObjectStoreErrorBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseS3Error("list_multipart_uploads", resp.StatusCode, resp.Header, body)
	}
	var response struct {
		Uploads []struct {
			UploadID string `xml:"UploadId"`
		} `xml:"Upload"`
	}
	if err := xml.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(response.Uploads))
	for _, upload := range response.Uploads {
		ids = append(ids, strings.TrimSpace(upload.UploadID))
	}
	return ids, nil
}
