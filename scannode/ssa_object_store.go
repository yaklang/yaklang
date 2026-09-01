package scannode

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultSSAObjectStoreRequestTimeout  = 5 * time.Minute
	defaultSSAObjectStoreFinalizeTimeout = 30 * time.Second
	defaultSSAObjectStoreRetryLimit      = 3
	maxSSAObjectStoreErrorBodyBytes      = 64 * 1024
	maxSSACertificateFileBytes           = 1024 * 1024
	maxSSAMultipartParts                 = 10000
)

// secretValue deliberately redacts itself from every normal formatting and
// JSON path. raw is only used at the HTTP signing boundary.
type secretValue string

func newSecretValue(value string) secretValue { return secretValue(strings.TrimSpace(value)) }
func (s secretValue) raw() string             { return string(s) }
func (s secretValue) String() string          { return "[REDACTED]" }
func (s secretValue) GoString() string        { return "[REDACTED]" }
func (s secretValue) MarshalText() ([]byte, error) {
	return []byte("[REDACTED]"), nil
}
func (s secretValue) MarshalJSON() ([]byte, error) {
	return json.Marshal("[REDACTED]")
}
func (s *secretValue) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("nil secret destination")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = newSecretValue(value)
	return nil
}

type objectStoreCredentials struct {
	AccessKey    secretValue
	SecretKey    secretValue
	SessionToken secretValue
}

type PutRequest struct {
	Bucket        string
	ObjectKey     string
	Body          io.ReadSeeker
	Size          int64
	ContentType   string
	PayloadSHA256 string
}

type CreateRequest struct {
	Bucket      string
	ObjectKey   string
	ContentType string
}

type PartRequest struct {
	Bucket        string
	ObjectKey     string
	UploadID      string
	PartNumber    int
	Body          io.ReadSeeker
	Size          int64
	PayloadSHA256 string
}

type CompletePart struct {
	PartNumber int
	ETag       string
}

type CompleteRequest struct {
	Bucket    string
	ObjectKey string
	UploadID  string
	Parts     []CompletePart
}

type AbortRequest struct {
	Bucket    string
	ObjectKey string
	UploadID  string
}

type ObjectStoreResult struct {
	ETag      string
	VersionID string
	RequestID string
}

// ObjectStoreUploader is the complete object-storage surface required by the
// ScanNode artifact pipeline. It intentionally excludes discovery, listing,
// downloads, and bucket administration.
type ObjectStoreUploader interface {
	Put(context.Context, PutRequest) (ObjectStoreResult, error)
	CreateMultipart(context.Context, CreateRequest) (string, ObjectStoreResult, error)
	UploadPart(context.Context, PartRequest) (string, ObjectStoreResult, error)
	CompleteMultipart(context.Context, CompleteRequest) (ObjectStoreResult, error)
	AbortMultipart(context.Context, AbortRequest) error
}

type ObjectStoreError struct {
	Operation  string
	Code       string
	Message    string
	StatusCode int
	RequestID  string
	Retryable  bool
	Cause      error
}

func (e *ObjectStoreError) Error() string {
	if e == nil {
		return "object store error"
	}
	parts := []string{"object store " + strings.TrimSpace(e.Operation) + " failed"}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.StatusCode != 0 {
		parts = append(parts, "status="+strconv.Itoa(e.StatusCode))
	}
	if e.RequestID != "" {
		parts = append(parts, "request_id="+e.RequestID)
	}
	if e.Message != "" {
		parts = append(parts, "message="+sanitizeObjectStoreMessage(e.Message))
	} else if e.Cause != nil {
		parts = append(parts, "cause="+sanitizeObjectStoreMessage(e.Cause.Error()))
	}
	return strings.Join(parts, " ")
}

func (e *ObjectStoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func sanitizeObjectStoreMessage(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if len(value) > 512 {
		value = value[:512]
	}
	return value
}

func isObjectStoreCredentialExpired(err error) bool {
	var storeErr *ObjectStoreError
	if !errors.As(err, &storeErr) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(storeErr.Code)) {
	case "expiredtoken", "invalidtoken", "invalidsecuritytoken", "tokenrefreshrequired":
		return true
	default:
		return false
	}
}

func classifyObjectStoreError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "upload_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upload_timeout"
	}
	var storeErr *ObjectStoreError
	if !errors.As(err, &storeErr) {
		return "put_failed"
	}
	if isObjectStoreCredentialExpired(storeErr) {
		return "sts_expired"
	}
	switch strings.ToLower(strings.TrimSpace(storeErr.Code)) {
	case "accessdenied", "allaccessdisabled":
		return "permission_denied"
	case "signaturedoesnotmatch", "authorizationheadermalformed", "invalidaccesskeyid":
		return "signature_invalid"
	case "nosuchbucket":
		return "bucket_not_found"
	case "invalidbucketname", "invalidargument", "invalidrequest", "keytoolongerror":
		return "invalid_upload_target"
	}
	switch storeErr.Operation {
	case "create_multipart", "upload_part", "complete_multipart", "abort_multipart":
		return "multipart_failed"
	default:
		return "put_failed"
	}
}

type s3ObjectStoreClient struct {
	endpoint         *url.URL
	region           string
	virtualHostStyle bool
	httpClient       *http.Client
	transport        *http.Transport
	now              func() time.Time
	retryLimit       int
	onRetry          func()

	credentialsMu sync.RWMutex
	credentials   objectStoreCredentials
}

func newS3ObjectStoreClient(cfg *SSAArtifactUploadConfig) (*s3ObjectStoreClient, error) {
	endpoint, err := validateSSAUploadConfig(cfg)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := buildSSAObjectStoreTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	transport.IdleConnTimeout = 90 * time.Second
	client := &s3ObjectStoreClient{
		endpoint:         endpoint,
		region:           normalizedSSARegion(cfg.Region),
		virtualHostStyle: cfg.VirtualHostStyle,
		httpClient: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		transport:  transport,
		now:        time.Now,
		retryLimit: defaultSSAObjectStoreRetryLimit,
	}
	client.setCredentials(credentialsFromSSAConfig(cfg))
	return client, nil
}

func (c *s3ObjectStoreClient) Close() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

func credentialsFromSSAConfig(cfg *SSAArtifactUploadConfig) objectStoreCredentials {
	if cfg == nil {
		return objectStoreCredentials{}
	}
	return objectStoreCredentials{
		AccessKey:    cfg.STSAccessKey,
		SecretKey:    cfg.STSSecretKey,
		SessionToken: cfg.STSSessionToken,
	}
}

func (c *s3ObjectStoreClient) setCredentials(credentials objectStoreCredentials) {
	c.credentialsMu.Lock()
	c.credentials = credentials
	c.credentialsMu.Unlock()
}

func (c *s3ObjectStoreClient) currentCredentials() objectStoreCredentials {
	c.credentialsMu.RLock()
	defer c.credentialsMu.RUnlock()
	return c.credentials
}

func buildSSAObjectStoreTLSConfig(cfg *SSAArtifactUploadConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg == nil || !cfg.TLSVerify,
	}
	caFile := ""
	if cfg != nil {
		caFile = strings.TrimSpace(cfg.TLSCAFile)
	}
	if caFile == "" {
		caFile = strings.TrimSpace(os.Getenv("SCANNODE_SSA_TLS_CA_FILE"))
	}
	if caFile == "" {
		return tlsConfig, nil
	}
	pemBytes, err := readSSACertificateFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read object store CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("object store CA file contains no certificates")
	}
	tlsConfig.RootCAs = pool
	return tlsConfig, nil
}

func (c *s3ObjectStoreClient) Put(ctx context.Context, req PutRequest) (ObjectStoreResult, error) {
	if req.Body == nil || req.Size < 0 {
		return ObjectStoreResult{}, &ObjectStoreError{Operation: "put", Code: "InvalidArgument", Message: "body and non-negative size required"}
	}
	return c.execute(ctx, "put", http.MethodPut, req.Bucket, req.ObjectKey, nil, req.ContentType, req.PayloadSHA256, req.Body, req.Size)
}

func (c *s3ObjectStoreClient) CreateMultipart(ctx context.Context, req CreateRequest) (string, ObjectStoreResult, error) {
	result, body, err := c.executeXML(ctx, "create_multipart", http.MethodPost, req.Bucket, req.ObjectKey, []queryValue{{Key: "uploads"}}, req.ContentType, emptySHA256Hex, nil, 0)
	if err != nil {
		return "", result, err
	}
	var response struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(body, &response); err != nil || strings.TrimSpace(response.UploadID) == "" {
		return "", result, &ObjectStoreError{Operation: "create_multipart", Code: "InvalidResponse", Message: "missing upload id", Cause: err}
	}
	return strings.TrimSpace(response.UploadID), result, nil
}

func (c *s3ObjectStoreClient) UploadPart(ctx context.Context, req PartRequest) (string, ObjectStoreResult, error) {
	if req.PartNumber < 1 || req.PartNumber > maxSSAMultipartParts || strings.TrimSpace(req.UploadID) == "" {
		return "", ObjectStoreResult{}, &ObjectStoreError{Operation: "upload_part", Code: "InvalidArgument", Message: "invalid part number or upload id"}
	}
	query := []queryValue{{Key: "partNumber", Value: strconv.Itoa(req.PartNumber)}, {Key: "uploadId", Value: req.UploadID}}
	result, err := c.execute(ctx, "upload_part", http.MethodPut, req.Bucket, req.ObjectKey, query, "application/octet-stream", req.PayloadSHA256, req.Body, req.Size)
	if err != nil {
		return "", result, err
	}
	if strings.TrimSpace(result.ETag) == "" {
		return "", result, &ObjectStoreError{Operation: "upload_part", Code: "InvalidResponse", Message: "missing ETag", RequestID: result.RequestID}
	}
	return result.ETag, result, nil
}

func (c *s3ObjectStoreClient) CompleteMultipart(ctx context.Context, req CompleteRequest) (ObjectStoreResult, error) {
	if strings.TrimSpace(req.UploadID) == "" || len(req.Parts) == 0 || len(req.Parts) > maxSSAMultipartParts {
		return ObjectStoreResult{}, &ObjectStoreError{Operation: "complete_multipart", Code: "InvalidArgument", Message: "upload id and parts required"}
	}
	type completedPartXML struct {
		PartNumber int    `xml:"PartNumber"`
		ETag       string `xml:"ETag"`
	}
	type completeUploadXML struct {
		XMLName xml.Name           `xml:"CompleteMultipartUpload"`
		Parts   []completedPartXML `xml:"Part"`
	}
	payload := completeUploadXML{Parts: make([]completedPartXML, 0, len(req.Parts))}
	for index, part := range req.Parts {
		if part.PartNumber != index+1 || strings.TrimSpace(part.ETag) == "" {
			return ObjectStoreResult{}, &ObjectStoreError{Operation: "complete_multipart", Code: "InvalidPart", Message: "parts must be ordered, contiguous, and include ETags"}
		}
		payload.Parts = append(payload.Parts, completedPartXML{PartNumber: part.PartNumber, ETag: quoteETag(part.ETag)})
	}
	body, err := xml.Marshal(payload)
	if err != nil {
		return ObjectStoreResult{}, err
	}
	payloadHash := sha256Hex(body)
	limit := c.retryLimit
	if limit < 1 {
		limit = 1
	}
	for attempt := 0; attempt < limit; attempt++ {
		result, responseBody, executeErr := c.executeXML(ctx, "complete_multipart", http.MethodPost, req.Bucket, req.ObjectKey, []queryValue{{Key: "uploadId", Value: req.UploadID}}, "application/xml", payloadHash, bytes.NewReader(body), int64(len(body)))
		if executeErr != nil {
			return result, executeErr
		}
		if embeddedErr := parseS3EmbeddedCompleteError(responseBody); embeddedErr != nil {
			if attempt+1 < limit && shouldRetryObjectStoreError(embeddedErr) {
				if c.onRetry != nil {
					c.onRetry()
				}
				if waitErr := waitSSAObjectStoreRetry(ctx, attempt); waitErr != nil {
					return result, waitErr
				}
				continue
			}
			return result, embeddedErr
		}
		var response struct {
			ETag string `xml:"ETag"`
		}
		if xml.Unmarshal(responseBody, &response) == nil && response.ETag != "" {
			result.ETag = unquoteETag(response.ETag)
		}
		return result, nil
	}
	return ObjectStoreResult{}, &ObjectStoreError{Operation: "complete_multipart", Code: "RetryLimitExceeded"}
}

func parseS3EmbeddedCompleteError(body []byte) error {
	parsed := struct {
		XMLName   xml.Name
		Code      string `xml:"Code"`
		Message   string `xml:"Message"`
		RequestID string `xml:"RequestId"`
	}{}
	if err := xml.Unmarshal(body, &parsed); err != nil || !strings.EqualFold(parsed.XMLName.Local, "Error") {
		return nil
	}
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		code = "InvalidResponse"
	}
	return &ObjectStoreError{
		Operation: "complete_multipart", Code: code, Message: parsed.Message,
		StatusCode: http.StatusOK, RequestID: strings.TrimSpace(parsed.RequestID), Retryable: isRetryableS3Response(http.StatusOK, code),
	}
}

func (c *s3ObjectStoreClient) AbortMultipart(ctx context.Context, req AbortRequest) error {
	if strings.TrimSpace(req.UploadID) == "" {
		return &ObjectStoreError{Operation: "abort_multipart", Code: "InvalidArgument", Message: "upload id required"}
	}
	_, err := c.execute(ctx, "abort_multipart", http.MethodDelete, req.Bucket, req.ObjectKey, []queryValue{{Key: "uploadId", Value: req.UploadID}}, "", emptySHA256Hex, nil, 0)
	return err
}

type queryValue struct {
	Key   string
	Value string
}

func (c *s3ObjectStoreClient) execute(ctx context.Context, operation, method, bucket, objectKey string, query []queryValue, contentType, payloadHash string, body io.ReadSeeker, size int64) (ObjectStoreResult, error) {
	result, _, err := c.executeXML(ctx, operation, method, bucket, objectKey, query, contentType, payloadHash, body, size)
	return result, err
}

func (c *s3ObjectStoreClient) executeXML(ctx context.Context, operation, method, bucket, objectKey string, query []queryValue, contentType, payloadHash string, body io.ReadSeeker, size int64) (ObjectStoreResult, []byte, error) {
	if c == nil || c.httpClient == nil || c.endpoint == nil {
		return ObjectStoreResult{}, nil, &ObjectStoreError{Operation: operation, Code: "ClientUnavailable"}
	}
	if err := validateSSABucket(bucket); err != nil {
		return ObjectStoreResult{}, nil, &ObjectStoreError{Operation: operation, Code: "InvalidBucketName", Message: err.Error()}
	}
	if err := validateSSAObjectKey(objectKey); err != nil {
		return ObjectStoreResult{}, nil, &ObjectStoreError{Operation: operation, Code: "InvalidArgument", Message: err.Error()}
	}
	if payloadHash == "" {
		return ObjectStoreResult{}, nil, &ObjectStoreError{Operation: operation, Code: "InvalidArgument", Message: "payload SHA-256 required"}
	}

	startOffset := int64(0)
	if body != nil {
		var err error
		startOffset, err = body.Seek(0, io.SeekCurrent)
		if err != nil {
			return ObjectStoreResult{}, nil, &ObjectStoreError{Operation: operation, Code: "InvalidArgument", Message: "request body must be seekable", Cause: err}
		}
	}
	limit := c.retryLimit
	if limit < 1 {
		limit = 1
	}
	var lastErr error
	for attempt := 0; attempt < limit; attempt++ {
		if err := ctx.Err(); err != nil {
			return ObjectStoreResult{}, nil, err
		}
		if body != nil {
			if _, err := body.Seek(startOffset, io.SeekStart); err != nil {
				return ObjectStoreResult{}, nil, err
			}
		}
		requestURL := c.objectURL(bucket, objectKey, query)
		var requestBody io.Reader
		if body != nil {
			requestBody = io.LimitReader(body, size)
		}
		req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), requestBody)
		if err != nil {
			return ObjectStoreResult{}, nil, err
		}
		if body != nil {
			req.ContentLength = size
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		credentials := c.currentCredentials()
		if err := signS3Request(req, credentials, c.region, payloadHash, c.now().UTC()); err != nil {
			return ObjectStoreResult{}, nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = classifyS3TransportError(operation, err)
			if attempt+1 < limit && shouldRetryObjectStoreError(lastErr) {
				if c.onRetry != nil {
					c.onRetry()
				}
				if err := waitSSAObjectStoreRetry(ctx, attempt); err != nil {
					return ObjectStoreResult{}, nil, err
				}
				continue
			}
			return ObjectStoreResult{}, nil, lastErr
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSSAObjectStoreErrorBodyBytes+1))
		_ = resp.Body.Close()
		result := ObjectStoreResult{
			ETag:      unquoteETag(resp.Header.Get("ETag")),
			VersionID: strings.TrimSpace(resp.Header.Get("x-amz-version-id")),
			RequestID: firstNonEmptyS3(resp.Header.Get("x-amz-request-id"), resp.Header.Get("x-minio-deployment-id")),
		}
		if readErr != nil {
			lastErr = &ObjectStoreError{Operation: operation, Code: "ResponseReadError", RequestID: result.RequestID, Retryable: true, Cause: readErr}
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = redactObjectStoreCredentials(parseS3Error(operation, resp.StatusCode, resp.Header, responseBody), credentials)
		} else {
			return result, responseBody, nil
		}
		if attempt+1 < limit && shouldRetryObjectStoreError(lastErr) {
			if c.onRetry != nil {
				c.onRetry()
			}
			if err := waitSSAObjectStoreRetry(ctx, attempt); err != nil {
				return ObjectStoreResult{}, nil, err
			}
			continue
		}
		return result, nil, lastErr
	}
	return ObjectStoreResult{}, nil, lastErr
}

func redactObjectStoreCredentials(err error, credentials objectStoreCredentials) error {
	var storeErr *ObjectStoreError
	if err == nil || !errors.As(err, &storeErr) {
		return err
	}
	for _, secret := range []secretValue{credentials.AccessKey, credentials.SecretKey, credentials.SessionToken} {
		if raw := secret.raw(); raw != "" {
			storeErr.Message = strings.ReplaceAll(storeErr.Message, raw, "[REDACTED]")
		}
	}
	return err
}

func (c *s3ObjectStoreClient) objectURL(bucket, objectKey string, query []queryValue) *url.URL {
	result := *c.endpoint
	basePath := strings.TrimSuffix(result.Path, "/")
	if c.virtualHostStyle {
		result.Host = bucket + "." + result.Host
		result.Path = basePath + "/" + objectKey
	} else {
		result.Path = basePath + "/" + bucket + "/" + objectKey
	}
	result.RawPath = awsURIEncode(result.Path, false)
	result.RawQuery = canonicalS3Query(query)
	return &result
}

func signS3Request(req *http.Request, credentials objectStoreCredentials, region, payloadHash string, now time.Time) error {
	if req == nil || req.URL == nil {
		return &ObjectStoreError{Operation: "sign", Code: "InvalidArgument", Message: "request URL required"}
	}
	accessKey := credentials.AccessKey.raw()
	secretKey := credentials.SecretKey.raw()
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return &ObjectStoreError{Operation: "sign", Code: "CredentialsMissing"}
	}
	region = normalizedSSARegion(region)
	payloadHash = strings.ToLower(strings.TrimSpace(payloadHash))
	if len(payloadHash) != sha256.Size*2 {
		return &ObjectStoreError{Operation: "sign", Code: "InvalidArgument", Message: "invalid payload SHA-256"}
	}
	now = now.UTC()
	amzDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if token := credentials.SessionToken.raw(); token != "" {
		req.Header.Set("x-amz-security-token", token)
	}
	canonicalHeaders, signedHeaders := canonicalS3Headers(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		awsURIEncode(req.URL.Path, false),
		canonicalizeRawQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	scope := shortDate + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex([]byte(canonicalRequest))
	kDate := hmacSHA256([]byte("AWS4"+secretKey), shortDate)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+",SignedHeaders="+signedHeaders+",Signature="+signature)
	return nil
}

func canonicalS3Headers(req *http.Request) (string, string) {
	headers := make(map[string][]string, len(req.Header)+1)
	headers["host"] = []string{req.URL.Host}
	for name, values := range req.Header {
		lowerName := strings.ToLower(strings.TrimSpace(name))
		switch lowerName {
		case "authorization", "content-length", "expect", "transfer-encoding", "user-agent", "x-amzn-trace-id":
			continue
		}
		for _, value := range values {
			headers[lowerName] = append(headers[lowerName], collapseHTTPSpaces(value))
		}
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteByte(':')
		canonical.WriteString(strings.Join(headers[name], ","))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(names, ";")
}

func collapseHTTPSpaces(value string) string { return strings.Join(strings.Fields(value), " ") }

func awsURIEncode(value string, encodeSlash bool) string {
	const hexUpper = "0123456789ABCDEF"
	var encoded strings.Builder
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' || (ch == '/' && !encodeSlash) {
			encoded.WriteByte(ch)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexUpper[ch>>4])
		encoded.WriteByte(hexUpper[ch&0x0f])
	}
	return encoded.String()
}

func canonicalS3Query(values []queryValue) string {
	sorted := append([]queryValue(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		leftKey, rightKey := awsURIEncode(sorted[i].Key, true), awsURIEncode(sorted[j].Key, true)
		if leftKey == rightKey {
			return awsURIEncode(sorted[i].Value, true) < awsURIEncode(sorted[j].Value, true)
		}
		return leftKey < rightKey
	})
	parts := make([]string, 0, len(sorted))
	for _, item := range sorted {
		parts = append(parts, awsURIEncode(item.Key, true)+"="+awsURIEncode(item.Value, true))
	}
	return strings.Join(parts, "&")
}

func canonicalizeRawQuery(values url.Values) string {
	items := make([]queryValue, 0)
	for key, list := range values {
		if len(list) == 0 {
			items = append(items, queryValue{Key: key})
			continue
		}
		for _, value := range list {
			items = append(items, queryValue{Key: key, Value: value})
		}
	}
	return canonicalS3Query(items)
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

const emptySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func parseS3Error(operation string, statusCode int, headers http.Header, body []byte) error {
	parsed := struct {
		Code      string `xml:"Code"`
		Message   string `xml:"Message"`
		RequestID string `xml:"RequestId"`
	}{}
	_ = xml.Unmarshal(body, &parsed)
	requestID := firstNonEmptyS3(parsed.RequestID, headers.Get("x-amz-request-id"), headers.Get("x-minio-deployment-id"))
	code := strings.TrimSpace(parsed.Code)
	if code == "" {
		code = http.StatusText(statusCode)
	}
	return &ObjectStoreError{
		Operation:  operation,
		Code:       code,
		Message:    parsed.Message,
		StatusCode: statusCode,
		RequestID:  strings.TrimSpace(requestID),
		Retryable:  isRetryableS3Response(statusCode, code),
	}
}

func classifyS3TransportError(operation string, err error) error {
	retryable := false
	var netErr net.Error
	if errors.As(err, &netErr) {
		retryable = netErr.Timeout() || netErr.Temporary()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		retryable = true
	}
	return &ObjectStoreError{Operation: operation, Code: "NetworkError", Retryable: retryable, Cause: err}
}

func shouldRetryObjectStoreError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var storeErr *ObjectStoreError
	return errors.As(err, &storeErr) && storeErr.Retryable
}

func isRetryableS3Response(statusCode int, code string) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "internalerror", "requesttimeout", "requesttimeoutexception", "serviceunavailable", "slowdown":
		return true
	default:
		return false
	}
}

func waitSSAObjectStoreRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(100*(1<<min(attempt, 4))) * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func quoteETag(value string) string   { return `"` + unquoteETag(value) + `"` }
func unquoteETag(value string) string { return strings.Trim(strings.TrimSpace(value), `"`) }

func firstNonEmptyS3(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizedSSARegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "us-east-1"
	}
	return region
}

func validateSSAUploadConfig(cfg *SSAArtifactUploadConfig) (*url.URL, error) {
	if cfg == nil {
		return nil, fmt.Errorf("empty upload config")
	}
	if err := validateSSABucket(cfg.Bucket); err != nil {
		return nil, err
	}
	if err := validateSSAObjectKey(cfg.ObjectKey); err != nil {
		return nil, err
	}
	region := normalizedSSARegion(cfg.Region)
	if len(region) > 64 {
		return nil, fmt.Errorf("invalid object store region")
	}
	for _, ch := range region {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			return nil, fmt.Errorf("invalid object store region")
		}
	}
	if cfg.STSAccessKey.raw() == "" || cfg.STSSecretKey.raw() == "" {
		return nil, fmt.Errorf("STS credentials missing")
	}
	endpointRaw := strings.TrimSpace(cfg.Endpoint)
	if endpointRaw == "" {
		return nil, fmt.Errorf("object store endpoint missing")
	}
	if !strings.Contains(endpointRaw, "://") {
		scheme := "http"
		if cfg.UseSSL {
			scheme = "https"
		}
		endpointRaw = scheme + "://" + endpointRaw
	}
	endpoint, err := url.Parse(endpointRaw)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, fmt.Errorf("invalid object store endpoint")
	}
	if endpoint.Scheme != "https" && endpoint.Scheme != "http" {
		return nil, fmt.Errorf("object store endpoint scheme must be HTTP or HTTPS")
	}
	if endpoint.Scheme == "http" && !cfg.AllowHTTP && !ssaExplicitHTTPAllowed() {
		return nil, fmt.Errorf("plaintext object store endpoint requires explicit development allowance")
	}
	if cfg.UseSSL && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("object store endpoint TLS mode mismatch")
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/")
	endpoint.RawPath = ""
	return endpoint, nil
}

func readSSACertificateFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, maxSSACertificateFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxSSACertificateFileBytes {
		return nil, fmt.Errorf("certificate file exceeds %d bytes", maxSSACertificateFileBytes)
	}
	return contents, nil
}

func validateSSABucket(bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if len(bucket) < 3 || len(bucket) > 63 {
		return fmt.Errorf("invalid object store bucket length")
	}
	if bucket[0] == '-' || bucket[len(bucket)-1] == '-' || bucket[0] == '.' || bucket[len(bucket)-1] == '.' || strings.Contains(bucket, "..") {
		return fmt.Errorf("invalid object store bucket")
	}
	for _, ch := range bucket {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.') {
			return fmt.Errorf("invalid object store bucket")
		}
	}
	if net.ParseIP(bucket) != nil {
		return fmt.Errorf("object store bucket cannot be an IP address")
	}
	return nil
}

func validateSSAObjectKey(objectKey string) error {
	if objectKey == "" || strings.TrimSpace(objectKey) == "" || len(objectKey) > 1024 || !utf8.ValidString(objectKey) {
		return fmt.Errorf("invalid object store object key")
	}
	if strings.HasPrefix(objectKey, "/") || strings.ContainsRune(objectKey, '\x00') {
		return fmt.Errorf("invalid object store object key")
	}
	for _, ch := range objectKey {
		if ch < 0x20 || ch == 0x7f {
			return fmt.Errorf("invalid object store object key")
		}
	}
	return nil
}
