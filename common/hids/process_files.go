package hids

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// GetProcessOpenFiles 获取进程打开的文件列表
// 在 Linux/macOS 上使用 /proc/PID/fd 或 lsof 命令
// Example:
// ```
// files = hids.GetProcessOpenFiles(1234)
//
//	for file in files {
//	    println(file)
//	}
//
// ```
func GetProcessOpenFiles(pid int) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var files []string

	switch runtime.GOOS {
	case "linux":
		// 使用 /proc/PID/fd
		files = getProcessOpenFilesByProc(ctx, pid)
	case "darwin":
		// macOS 使用 lsof 命令
		files = getProcessOpenFilesByLsof(ctx, pid)
	default:
		log.Warnf("unsupported OS for getting process open files: %s", runtime.GOOS)
		return nil
	}

	return files
}

// getProcessOpenFilesByProc 通过 /proc/PID/fd 获取进程打开的文件（Linux）
func getProcessOpenFilesByProc(ctx context.Context, pid int) []string {
	fdPath := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	files, err := os.ReadDir(fdPath)
	if err != nil {
		return nil
	}

	var openFiles []string
	for _, file := range files {
		fdLink := filepath.Join(fdPath, file.Name())
		target, err := os.Readlink(fdLink)
		if err != nil {
			continue
		}

		// 过滤掉 socket、pipe 等特殊文件
		if !strings.HasPrefix(target, "socket:") &&
			!strings.HasPrefix(target, "pipe:") &&
			!strings.HasPrefix(target, "anon_inode:") {
			openFiles = append(openFiles, target)
		}
	}

	return openFiles
}

// getProcessOpenFilesByLsof 通过 lsof 命令获取进程打开的文件（macOS）
func getProcessOpenFilesByLsof(ctx context.Context, pid int) []string {
	cmd := exec.CommandContext(ctx, "lsof", "-p", strconv.Itoa(pid), "-F", "n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	var openFiles []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			file := line[1:]
			// 过滤掉 socket、pipe 等
			if !strings.HasPrefix(file, "socket:") &&
				!strings.HasPrefix(file, "pipe:") &&
				!strings.Contains(file, "->") {
				openFiles = append(openFiles, file)
			}
		}
	}

	return openFiles
}

// GetProcessOpenFilesCount 获取进程打开的文件数量
// Example:
// ```
// count = hids.GetProcessOpenFilesCount(1234)
// println(count)
// ```
func GetProcessOpenFilesCount(pid int) int {
	files := GetProcessOpenFiles(pid)
	return len(files)
}

// UpdateProcessOpenFiles 更新进程信息中的打开文件列表
func UpdateProcessOpenFiles(proc *ProcessInfo) {
	if proc == nil {
		return
	}

	files := GetProcessOpenFiles(proc.Pid)
	proc.OpenFiles = files
	proc.OpenFilesCount = len(files)
}
