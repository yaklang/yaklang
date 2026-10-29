//go:build hids && linux

package ebpf

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	gopscommon "github.com/shirou/gopsutil/v4/common"
	gopsprocess "github.com/shirou/gopsutil/v4/process"
)

type processEventMetadata struct {
	parentPID  int
	parentName string
	name       string
	username   string
	image      string
	command    string
	status     string
}

// Read a fresh identity for this event. NewProcess and Ppid also calculate
// process creation time (including host boot time), which these events do not
// use. Reading the needed status fields together avoids that work and repeated
// status reads without caching an identity across PID reuse or exec.
func readProcessEventMetadata(ctx context.Context, pid int) (processEventMetadata, bool) {
	var info processEventMetadata
	if pid <= 0 || ctx.Err() != nil {
		return info, false
	}
	processRoot := filepath.Join(procRootWithContext(ctx), strconv.Itoa(pid))
	statusRaw, statusErr := os.ReadFile(filepath.Join(processRoot, "status"))
	commRaw, commErr := os.ReadFile(filepath.Join(processRoot, "comm"))
	if statusErr != nil && commErr != nil {
		return info, false
	}

	statusName, uid := "", ""
	for _, line := range strings.Split(string(statusRaw), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Name":
			statusName = value
		case "PPid", "Ppid":
			if parent, err := strconv.ParseInt(value, 10, 32); err == nil && parent > 0 {
				info.parentPID = int(parent)
			}
		case "Uid":
			if values := strings.Fields(value); len(values) > 0 {
				if number, err := strconv.ParseUint(values[0], 10, 32); err == nil {
					uid = strconv.FormatUint(number, 10)
				}
			}
		case "State":
			if value != "" {
				info.status = processStatusName(value[0])
			}
		}
	}
	if statusErr != nil {
		// Keep the old best-effort parent fallback when status is unavailable
		// but comm is readable. Only PPid is needed; gopsutil's Ppid getter
		// would also calculate unused creation/boot time from this stat file.
		info.parentPID = readParentPIDFromStat(processRoot)
	}
	if uid != "" {
		if account, err := user.LookupId(uid); err == nil {
			info.username = account.Username
		}
	}

	// Keep gopsutil's comm/name and NUL-separated cmdline semantics. Reuse the
	// same cmdline read when a 15-byte comm needs its executable-name extension.
	cmdlineRaw, _ := os.ReadFile(filepath.Join(processRoot, "cmdline"))
	arguments := strings.FieldsFunc(string(cmdlineRaw), func(r rune) bool { return r == 0 })
	info.command = strings.Join(arguments, " ")
	info.name = strings.TrimSuffix(string(commRaw), "\n")
	if info.name == "" || len(info.name) >= 15 {
		info.name = statusName
		if len(info.name) >= 15 && len(cmdlineRaw) > 0 {
			argument, _, _ := strings.Cut(string(cmdlineRaw), "\x00")
			if extended := filepath.Base(argument); strings.HasPrefix(extended, info.name) {
				info.name = extended
			}
		}
	}
	info.image, _ = os.Readlink(filepath.Join(processRoot, "exe"))
	if info.parentPID > 0 && ctx.Err() == nil {
		// A Process value alone does no proc/boot-time I/O. Name only reads the
		// parent's comm/status, retaining its long-name handling.
		parent := &gopsprocess.Process{Pid: int32(info.parentPID)}
		info.parentName, _ = parent.NameWithContext(ctx)
		info.parentName = strings.TrimSpace(info.parentName)
	}
	return info, true
}

func readParentPIDFromStat(processRoot string) int {
	raw, err := os.ReadFile(filepath.Join(processRoot, "stat"))
	if err != nil {
		return 0
	}
	// comm is parenthesized and may itself contain spaces and parentheses.
	// Fields after its final ')' start with state, then PPid.
	closing := strings.LastIndexByte(string(raw), ')')
	if closing < 0 {
		return 0
	}
	fields := strings.Fields(string(raw[closing+1:]))
	if len(fields) < 2 {
		return 0
	}
	parent, err := strconv.ParseInt(fields[1], 10, 32)
	if err != nil || parent <= 0 {
		return 0
	}
	return int(parent)
}

func procRootWithContext(ctx context.Context) string {
	if environment, ok := ctx.Value(gopscommon.EnvKey).(gopscommon.EnvMap); ok {
		if root := environment[gopscommon.HostProcEnvKey]; root != "" {
			return root
		}
	}
	if root := os.Getenv(string(gopscommon.HostProcEnvKey)); root != "" {
		return root
	}
	return "/proc"
}

// Match gopsutil's public process status values, preserving event fields.
func processStatusName(state byte) string {
	switch state {
	case 'A':
		return gopsprocess.Daemon
	case 'D', 'U':
		return gopsprocess.Blocked
	case 'E':
		return gopsprocess.Detached
	case 'I':
		return gopsprocess.Idle
	case 'L':
		return gopsprocess.Lock
	case 'O':
		return gopsprocess.Orphan
	case 'R':
		return gopsprocess.Running
	case 'S':
		return gopsprocess.Sleep
	case 'T', 't':
		return gopsprocess.Stop
	case 'W':
		return gopsprocess.Wait
	case 'Y':
		return gopsprocess.System
	case 'Z':
		return gopsprocess.Zombie
	default:
		return gopsprocess.UnknownState
	}
}
