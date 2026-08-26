package ssaapi

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/yaklang/yaklang/common/utils/yakgit"
)

const (
	ssaGitCloneMaxAttempts = 3
	ssaGitCloneRetryBase   = time.Second
)

// cloneSSAGitRepositoryWithRetry clones into local and retries transient network
// failures. Partial clone debris is cleared between attempts so go-git can reuse
// the same workspace directory.
func cloneSSAGitRepositoryWithRetry(
	url string,
	local string,
	opts []yakgit.Option,
	report func(format string, args ...any),
) error {
	var lastErr error
	for attempt := 1; attempt <= ssaGitCloneMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := resetCloneWorkspace(local); err != nil {
				return fmt.Errorf("reset SSA Git workspace before retry: %w", err)
			}
			delay := ssaGitCloneRetryBase * time.Duration(1<<(attempt-2))
			if report != nil {
				report("git clone retry %d/%d after %s: %v", attempt, ssaGitCloneMaxAttempts, delay, lastErr)
			}
			log.Warnf(
				"SSA Git clone retry %d/%d after %s for %s: %v",
				attempt,
				ssaGitCloneMaxAttempts,
				delay,
				url,
				lastErr,
			)
			ssaGitCloneSleep(delay)
		}

		err := cloneSSAGitRepository(url, local, opts...)
		if err == nil {
			if attempt > 1 && report != nil {
				report("git clone succeeded on attempt %d/%d", attempt, ssaGitCloneMaxAttempts)
			}
			return nil
		}
		lastErr = err
		if !isRetryableGitCloneError(err) || attempt == ssaGitCloneMaxAttempts {
			return err
		}
	}
	return lastErr
}

func resetCloneWorkspace(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.MkdirAll(dir, 0o700)
		}
		return err
	}
	var joinErr error
	for _, entry := range entries {
		joinErr = errors.Join(joinErr, os.RemoveAll(filepath.Join(dir, entry.Name())))
	}
	return joinErr
}

func isRetryableGitCloneError(err error) bool {
	if err == nil {
		return false
	}
	// EOF / unexpected EOF are transient at the transport edge.
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed) ||
		errors.Is(err, transport.ErrRepositoryNotFound) ||
		errors.Is(err, transport.ErrEmptyRemoteRepository) ||
		errors.Is(err, syscall.ENOSPC) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	message := strings.ToLower(err.Error())
	permanent := []string{
		"authentication required",
		"authorization failed",
		"repository not found",
		"empty repository",
		"no space left on device",
		"disk quota exceeded",
		"invalid proxy url",
		"remote repository is empty",
	}
	for _, needle := range permanent {
		if strings.Contains(message, needle) {
			return false
		}
	}
	transient := []string{
		"eof",
		"tls handshake timeout",
		"i/o timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"server misbehaving",
		"broken pipe",
		"http2: client connection lost",
		"use of closed network connection",
		"wsarecv",
		"network is unreachable",
	}
	for _, needle := range transient {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

// cloneSSAGitRepositoryPreferShallow tries a depth-1 (or default) clone first,
// then falls back to Depth(0) when the remote rejects shallow fetch. Permanent
// auth/not-found/disk errors are not retried as full clones.
func cloneSSAGitRepositoryPreferShallow(
	url string,
	local string,
	opts []yakgit.Option,
	report func(format string, args ...any),
) error {
	err := cloneSSAGitRepositoryWithRetry(url, local, opts, report)
	if err == nil {
		return nil
	}
	if !shouldFallbackFromShallowClone(err) {
		return err
	}
	if report != nil {
		report("git clone shallow rejected; falling back to full clone: %v", err)
	}
	log.Warnf("SSA Git shallow clone rejected for %s; falling back to full clone: %v", url, err)
	if resetErr := resetCloneWorkspace(local); resetErr != nil {
		return fmt.Errorf("reset SSA Git workspace before full clone fallback: %w", resetErr)
	}
	return cloneSSAGitRepositoryWithRetry(url, local, withoutShallowCloneOptions(opts), report)
}

func shouldFallbackFromShallowClone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, transport.ErrAuthenticationRequired) ||
		errors.Is(err, transport.ErrAuthorizationFailed) ||
		errors.Is(err, transport.ErrRepositoryNotFound) ||
		errors.Is(err, transport.ErrEmptyRemoteRepository) ||
		errors.Is(err, syscall.ENOSPC) {
		return false
	}
	message := strings.ToLower(err.Error())
	permanent := []string{
		"authentication required",
		"authorization failed",
		"repository not found",
		"empty repository",
		"no space left on device",
		"disk quota exceeded",
		"invalid proxy url",
		"remote repository is empty",
	}
	for _, needle := range permanent {
		if strings.Contains(message, needle) {
			return false
		}
	}
	return true
}

func withoutShallowCloneOptions(opts []yakgit.Option) []yakgit.Option {
	// yakgit defaults Depth=1; append Depth(0) last so go-git does a full clone.
	return append(append([]yakgit.Option{}, opts...), yakgit.WithDepth(0))
}
