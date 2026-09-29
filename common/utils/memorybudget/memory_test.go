package memorybudget

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCgroupMemoryBudget(t *testing.T) {
	for _, tc := range []struct {
		name, groups, mounts string
		files                map[string]string
		want                 int64
	}{
		{"v2 parent limit", "0::/user/job", "1 0 0:1 / /cg rw - cgroup2 cgroup rw", map[string]string{"/cg/user/job/memory.max": "max", "/cg/user/memory.max": "1024"}, 1024},
		{"v2 throttle", "0::/job", "1 0 0:1 / /cg rw - cgroup2 cgroup rw", map[string]string{"/cg/job/memory.max": "4096", "/cg/job/memory.high": "2048"}, 2048},
		{"v1 mount root", "5:cpu,memory:/slice/job", "1 0 0:1 /slice /cg rw - cgroup cgroup rw,memory", map[string]string{"/cg/job/memory.limit_in_bytes": "512"}, 512},
		{"unlimited", "0::/job", "1 0 0:1 / /cg rw - cgroup2 cgroup rw", map[string]string{"/cg/job/memory.max": "max"}, 8192},
		{"outside mount", "0::/other/job", "1 0 0:1 /slice /cg rw - cgroup2 cgroup rw", map[string]string{"/cg/memory.max": "1"}, 8192},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := constrainedTotal(8192, tc.groups, tc.mounts, func(path string) ([]byte, error) {
				if v, ok := tc.files[path]; ok {
					return []byte(v), nil
				}
				return nil, os.ErrNotExist
			})
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPressureThresholdsScaleAndRespectRuntimeLimit(t *testing.T) {
	w32, c32 := PressureThresholds(32<<30, math.MaxInt64)
	w128, c128 := PressureThresholds(128<<30, math.MaxInt64)
	require.Greater(t, w32, int64(4<<30))
	require.InDelta(t, float64(w32)*4, float64(w128), 16)
	require.InDelta(t, float64(c32)*4, float64(c128), 16)
	w, c := PressureThresholds(128<<30, 1<<30)
	require.Equal(t, int64(768<<20), w)
	require.Less(t, c, int64(1<<30))
	w, c = PressureThresholds(0, math.MaxInt64)
	require.Equal(t, int64(2<<30), w)
	require.Equal(t, int64(4<<30), c)
}
