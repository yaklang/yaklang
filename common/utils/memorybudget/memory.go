// Package memorybudget reports physical memory constrained by the process's
// cgroup. It does not change GC settings or reserve memory.
package memorybudget

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
)

// PressureThresholds returns warning and critical managed-memory marks for a
// process whose host or cgroup capacity is total, further limited by an
// explicit Go memory limit. A non-positive total falls back to 2GiB/4GiB.
// runtimeLimit at or above MaxInt64 is treated as unset.
func PressureThresholds(total, runtimeLimit int64) (warning, critical int64) {
	// Reserve 20% of host/container RAM for non-Go allocations and other
	// processes; respect a tighter explicit or adaptive Go memory limit.
	budget := total / 5 * 4
	if runtimeLimit > 0 && runtimeLimit < int64(^uint64(0)>>1) && (budget <= 0 || runtimeLimit < budget) {
		budget = runtimeLimit
	}
	if budget <= 0 {
		return 2 << 30, 4 << 30
	}
	return budget / 4 * 3, budget / 10 * 9
}

func Total() int64 {
	var total int64
	if vm, err := mem.VirtualMemory(); err == nil {
		total = int64(vm.Total)
	}
	groups, _ := os.ReadFile("/proc/self/cgroup")
	mounts, _ := os.ReadFile("/proc/self/mountinfo")
	return constrainedTotal(total, string(groups), string(mounts), os.ReadFile)
}

func constrainedTotal(total int64, groups, mounts string, read func(string) ([]byte, error)) int64 {
	for _, group := range strings.Split(groups, "\n") {
		parts := strings.SplitN(group, ":", 3)
		if len(parts) != 3 {
			continue
		}
		v2 := parts[1] == ""
		if !v2 && !containsController(parts[1], "memory") {
			continue
		}
		for _, mount := range strings.Split(mounts, "\n") {
			left, right, ok := strings.Cut(mount, " - ")
			fields, fs := strings.Fields(left), strings.Fields(right)
			if !ok || len(fields) < 5 || len(fs) < 3 {
				continue
			}
			if v2 && fs[0] != "cgroup2" || !v2 && (fs[0] != "cgroup" || !containsController(fs[2], "memory")) {
				continue
			}
			root, mountpoint := unescapeMount(fields[3]), unescapeMount(fields[4])
			rel, err := filepath.Rel(root, filepath.Clean(parts[2]))
			if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				continue
			}
			dir := filepath.Join(mountpoint, rel)
			files := []string{"memory.max", "memory.high"}
			if !v2 {
				files = []string{"memory.limit_in_bytes"}
			}
			// Parent cgroups also constrain the process. Stop at the mount root.
			for {
				for _, file := range files {
					data, err := read(filepath.Join(dir, file))
					if err != nil {
						continue
					}
					limit, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
					if err == nil && limit > 0 && (total <= 0 || limit < total) {
						total = limit
					}
				}
				if dir == mountpoint || dir == filepath.Dir(dir) {
					break
				}
				dir = filepath.Dir(dir)
			}
		}
	}
	return total
}

func containsController(list, controller string) bool {
	for _, item := range strings.Split(list, ",") {
		if item == controller {
			return true
		}
	}
	return false
}

func unescapeMount(path string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(path)
}
