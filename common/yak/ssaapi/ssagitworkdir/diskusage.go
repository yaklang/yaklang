package ssagitworkdir

import "context"

type diskUsageStat struct {
	Free        uint64
	InodesTotal uint64
	InodesFree  uint64
}

func diskUsageForPath(ctx context.Context, path string) (*diskUsageStat, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return platformDiskUsage(path)
}
