//go:build windows
// +build windows

package healthinfo

import "context"

func DiskUsageWithContext(ctx context.Context, path string) (*UsageStat, error) {
	return nil, nil
}
