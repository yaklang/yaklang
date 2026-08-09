//go:build linux || darwin || freebsd

package ssagitworkdir

import "golang.org/x/sys/unix"

func platformDiskUsage(path string) (*diskUsageStat, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return nil, err
	}
	return &diskUsageStat{
		Free:        uint64(stat.Bavail) * uint64(stat.Bsize),
		InodesTotal: uint64(stat.Files),
		InodesFree:  uint64(stat.Ffree),
	}, nil
}
