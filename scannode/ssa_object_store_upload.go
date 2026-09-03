package scannode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type ssaObjectStoreUploadStats struct {
	Bytes     int64
	SHA256    string
	ETag      string
	RequestID string
	Parts     int
}

// ssaObjectStoreUploadSession owns exactly one S3 client and transport for an
// artifact task. Credential refresh updates only the signer state, preserving
// the transport and its connection pool.
type ssaObjectStoreUploadSession struct {
	provider          ssaUploadConfigProvider
	uploader          ObjectStoreUploader
	updateCredentials func(objectStoreCredentials)
	closeUploader     func()

	endpoint         string
	bucket           string
	region           string
	virtualHostStyle bool
	onRetry          func()
	credentialMu     sync.Mutex
	credentialGen    uint64
}

func newSSAObjectStoreUploadSession(provider ssaUploadConfigProvider, onRetry func()) (*ssaObjectStoreUploadSession, error) {
	if provider == nil {
		return nil, fmt.Errorf("empty upload config provider")
	}
	cfg, err := provider(false)
	if err != nil {
		return nil, err
	}
	endpoint, err := validateSSAUploadConfig(cfg)
	if err != nil {
		return nil, err
	}
	client, err := newS3ObjectStoreClient(cfg)
	if err != nil {
		return nil, err
	}
	client.onRetry = onRetry
	return &ssaObjectStoreUploadSession{
		provider:          provider,
		uploader:          client,
		updateCredentials: client.setCredentials,
		closeUploader:     client.Close,
		endpoint:          endpoint.String(),
		bucket:            strings.TrimSpace(cfg.Bucket),
		region:            normalizedSSARegion(cfg.Region),
		virtualHostStyle:  cfg.VirtualHostStyle,
		onRetry:           onRetry,
	}, nil
}

func (s *ssaObjectStoreUploadSession) Close() {
	if s != nil && s.closeUploader != nil {
		s.closeUploader()
	}
}

func (s *ssaObjectStoreUploadSession) refreshCredentials(force bool) error {
	if s == nil || s.provider == nil || s.uploader == nil || s.updateCredentials == nil {
		return fmt.Errorf("object store upload session unavailable")
	}
	cfg, err := s.provider(force)
	if err != nil {
		return err
	}
	endpoint, err := validateSSAUploadConfig(cfg)
	if err != nil {
		return err
	}
	if endpoint.String() != s.endpoint || strings.TrimSpace(cfg.Bucket) != s.bucket || normalizedSSARegion(cfg.Region) != s.region || cfg.VirtualHostStyle != s.virtualHostStyle {
		return fmt.Errorf("upload target changed during STS refresh")
	}
	s.updateCredentials(credentialsFromSSAConfig(cfg))
	return nil
}

func (s *ssaObjectStoreUploadSession) withCredentialRefresh(operation func() error) error {
	s.credentialMu.Lock()
	if err := s.refreshCredentials(false); err != nil {
		s.credentialMu.Unlock()
		return err
	}
	generation := s.credentialGen
	s.credentialMu.Unlock()
	err := operation()
	if !isObjectStoreCredentialExpired(err) {
		return err
	}
	if s.onRetry != nil {
		s.onRetry()
	}
	s.credentialMu.Lock()
	if generation == s.credentialGen {
		if refreshErr := s.refreshCredentials(true); refreshErr != nil {
			s.credentialMu.Unlock()
			return errors.Join(err, refreshErr)
		}
		s.credentialGen++
	}
	s.credentialMu.Unlock()
	return operation()
}

func (s *ssaObjectStoreUploadSession) uploadFile(ctx context.Context, path string, size int64, objectKey string) (ssaObjectStoreUploadStats, error) {
	file, err := os.Open(path)
	if err != nil {
		return ssaObjectStoreUploadStats{}, err
	}
	defer file.Close()
	if size < 0 {
		return ssaObjectStoreUploadStats{}, fmt.Errorf("artifact size cannot be negative")
	}
	if size == 0 {
		stat, statErr := file.Stat()
		if statErr != nil {
			return ssaObjectStoreUploadStats{}, statErr
		}
		size = stat.Size()
	}
	return s.upload(ctx, file, size, objectKey)
}

func (s *ssaObjectStoreUploadSession) uploadBytes(ctx context.Context, payload []byte, objectKey string) (ssaObjectStoreUploadStats, error) {
	return s.upload(ctx, bytes.NewReader(payload), int64(len(payload)), objectKey)
}

func (s *ssaObjectStoreUploadSession) upload(ctx context.Context, body io.Reader, size int64, objectKey string) (stats ssaObjectStoreUploadStats, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil || s.uploader == nil {
		return stats, fmt.Errorf("object store upload session unavailable")
	}
	if err := validateSSAObjectKey(objectKey); err != nil {
		return stats, err
	}
	partSize := readSSAMultipartPartSize()
	if size >= 0 {
		partCount := (size + partSize - 1) / partSize
		if partCount > maxSSAMultipartParts {
			return stats, fmt.Errorf("artifact requires %d parts; maximum is %d", partCount, maxSSAMultipartParts)
		}
	}
	if seeker, ok := body.(io.ReadSeeker); ok && size <= partSize && size >= 0 {
		payloadHash, hashErr := hashSSAUploadBody(seeker, size)
		if hashErr != nil {
			return stats, hashErr
		}
		bodyStart, seekErr := seeker.Seek(0, io.SeekCurrent)
		if seekErr != nil {
			return stats, seekErr
		}
		var result ObjectStoreResult
		err = s.withCredentialRefresh(func() error {
			if _, err := seeker.Seek(bodyStart, io.SeekStart); err != nil {
				return err
			}
			requestCtx, cancel := context.WithTimeout(ctx, readSSAObjectStoreRequestTimeout())
			defer cancel()
			var putErr error
			result, putErr = s.uploader.Put(requestCtx, PutRequest{
				Bucket:        s.bucket,
				ObjectKey:     objectKey,
				Body:          seeker,
				Size:          size,
				ContentType:   "application/octet-stream",
				PayloadSHA256: payloadHash,
			})
			return putErr
		})
		if err != nil {
			return stats, err
		}
		return ssaObjectStoreUploadStats{Bytes: size, SHA256: payloadHash, ETag: result.ETag, RequestID: result.RequestID}, nil
	}

	var uploadID string
	var createResult ObjectStoreResult
	err = s.withCredentialRefresh(func() error {
		requestCtx, cancel := context.WithTimeout(ctx, readSSAObjectStoreRequestTimeout())
		defer cancel()
		var createErr error
		uploadID, createResult, createErr = s.uploader.CreateMultipart(requestCtx, CreateRequest{
			Bucket:      s.bucket,
			ObjectKey:   objectKey,
			ContentType: "application/octet-stream",
		})
		return createErr
	})
	if err != nil {
		return stats, err
	}
	abortNeeded := true
	defer func() {
		if !abortNeeded {
			return
		}
		cleanupBase := context.WithoutCancel(ctx)
		cleanupCtx, cancel := context.WithTimeout(cleanupBase, readSSAObjectStoreFinalizeTimeout())
		defer cancel()
		abortErr := s.uploader.AbortMultipart(cleanupCtx, AbortRequest{Bucket: s.bucket, ObjectKey: objectKey, UploadID: uploadID})
		if abortErr != nil {
			err = errors.Join(err, fmt.Errorf("abort incomplete multipart upload: %w", abortErr))
		}
	}()

	// Multipart payload buffers are the dominant bounded allocation. The exact
	// upper bound is part_size * concurrency (default 16 MiB * 2 = 32 MiB;
	// configurable maximum 128 MiB * 2 = 256 MiB).
	concurrency := readSSAMultipartConcurrency()
	type multipartUploadJob struct {
		partNumber int
		buffer     []byte
		size       int
		sha256     string
	}
	uploadCtx, cancelUploads := context.WithCancel(ctx)
	defer cancelUploads()
	if closer, ok := body.(io.Closer); ok {
		stopReadCancel := make(chan struct{})
		defer close(stopReadCancel)
		go func() {
			select {
			case <-uploadCtx.Done():
				_ = closer.Close()
			case <-stopReadCancel:
			}
		}()
	}
	jobs := make(chan multipartUploadJob, concurrency)
	buffers := make(chan []byte, concurrency)
	for index := 0; index < concurrency; index++ {
		buffers <- make([]byte, partSize)
	}
	parts := make([]CompletePart, maxSSAMultipartParts)
	var workerWG sync.WaitGroup
	var resultMu sync.Mutex
	var firstPartErr error
	var partErrOnce sync.Once
	workerWG.Add(concurrency)
	for worker := 0; worker < concurrency; worker++ {
		go func() {
			defer workerWG.Done()
			for job := range jobs {
				if uploadCtx.Err() == nil {
					var etag string
					var partResult ObjectStoreResult
					partErr := s.withCredentialRefresh(func() error {
						requestCtx, cancel := context.WithTimeout(uploadCtx, readSSAObjectStoreRequestTimeout())
						defer cancel()
						var uploadErr error
						etag, partResult, uploadErr = s.uploader.UploadPart(requestCtx, PartRequest{
							Bucket:        s.bucket,
							ObjectKey:     objectKey,
							UploadID:      uploadID,
							PartNumber:    job.partNumber,
							Body:          bytes.NewReader(job.buffer[:job.size]),
							Size:          int64(job.size),
							PayloadSHA256: job.sha256,
						})
						return uploadErr
					})
					if partErr != nil {
						partErrOnce.Do(func() {
							firstPartErr = partErr
							cancelUploads()
						})
					} else {
						parts[job.partNumber-1] = CompletePart{PartNumber: job.partNumber, ETag: etag}
						resultMu.Lock()
						stats.ETag = etag
						stats.RequestID = firstNonEmptyS3(partResult.RequestID, stats.RequestID, createResult.RequestID)
						resultMu.Unlock()
					}
				}
				buffers <- job.buffer
			}
		}()
	}

	hasher := sha256.New()
	partCount := 0
	var readFailure error
readParts:
	for partNumber := 1; ; partNumber++ {
		if partNumber > maxSSAMultipartParts {
			readFailure = fmt.Errorf("multipart upload exceeds %d parts", maxSSAMultipartParts)
			break
		}
		var buffer []byte
		select {
		case buffer = <-buffers:
		case <-uploadCtx.Done():
			readFailure = uploadCtx.Err()
			break readParts
		}
		n, readErr := io.ReadFull(body, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			buffers <- buffer
			readFailure = readErr
			cancelUploads()
			break
		}
		if n > 0 {
			chunk := buffer[:n]
			_, _ = hasher.Write(chunk)
			stats.Bytes += int64(n)
			job := multipartUploadJob{partNumber: partNumber, buffer: buffer, size: n, sha256: sha256Hex(chunk)}
			select {
			case jobs <- job:
				partCount++
			case <-uploadCtx.Done():
				buffers <- buffer
				readFailure = uploadCtx.Err()
				break readParts
			}
		} else {
			buffers <- buffer
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	close(jobs)
	workerWG.Wait()
	if firstPartErr != nil {
		return stats, firstPartErr
	}
	if readFailure != nil {
		return stats, readFailure
	}
	if partCount == 0 {
		return stats, fmt.Errorf("no multipart parts uploaded")
	}
	parts = parts[:partCount]
	for index, part := range parts {
		if part.PartNumber != index+1 || strings.TrimSpace(part.ETag) == "" {
			return stats, fmt.Errorf("multipart part %d did not complete", index+1)
		}
	}
	stats.Parts = partCount
	stats.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	var completeResult ObjectStoreResult
	err = s.withCredentialRefresh(func() error {
		requestCtx, cancel := context.WithTimeout(ctx, readSSAObjectStoreFinalizeTimeout())
		defer cancel()
		var completeErr error
		completeResult, completeErr = s.uploader.CompleteMultipart(requestCtx, CompleteRequest{
			Bucket: s.bucket, ObjectKey: objectKey, UploadID: uploadID, Parts: parts,
		})
		return completeErr
	})
	if err != nil {
		return stats, err
	}
	abortNeeded = false
	stats.ETag = firstNonEmptyS3(completeResult.ETag, stats.ETag)
	stats.RequestID = firstNonEmptyS3(completeResult.RequestID, stats.RequestID, createResult.RequestID)
	return stats, nil
}

func hashSSAUploadBody(body io.ReadSeeker, size int64) (string, error) {
	start, err := body.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	written, copyErr := io.CopyN(hasher, body, size)
	_, seekErr := body.Seek(start, io.SeekStart)
	if copyErr != nil && !errors.Is(copyErr, io.EOF) {
		return "", copyErr
	}
	if written != size {
		return "", fmt.Errorf("upload body size mismatch: expected=%d actual=%d", size, written)
	}
	if seekErr != nil {
		return "", seekErr
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func readSSAObjectStoreRequestTimeout() time.Duration {
	return readPositiveDurationSeconds("SCANNODE_SSA_UPLOAD_TIMEOUT_SEC", defaultSSAObjectStoreRequestTimeout, 24*time.Hour)
}

func readSSAObjectStoreFinalizeTimeout() time.Duration {
	return readPositiveDurationSeconds("SCANNODE_SSA_FINALIZE_TIMEOUT_SEC", defaultSSAObjectStoreFinalizeTimeout, 10*time.Minute)
}

func readPositiveDurationSeconds(name string, fallback, maximum time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	seconds, err := time.ParseDuration(raw + "s")
	if err != nil || seconds <= 0 || seconds > maximum {
		return fallback
	}
	return seconds
}
