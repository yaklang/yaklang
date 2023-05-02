package guard

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

type MySQLServerDetail struct {
	Process *PsProcess
	MainPid int `json:"main_pid"`

	// language.*?PATH
	BaseDir        string `json:"base_dir"`
	DefaultRootDir string `json:"default_root_dir"`

	// mysql.*?Ver[\s]+%{DATA:full}[\s]+
	VersionFull string `json:"version_full"`
	// mysql.*?Ver[\s]+%{COMMONVERSION:full}[\s]+
	VersionShort string `json:"version_short"`

	// from process
	BinaryFile string `json:"binary_file"`

	// --default-files=PATH
	ConfigPath string `json:"config_path"`
	DataDir    string `json:"data_dir"`
}

func getMysqldProcess(c context.Context) []*PsProcess {
	cmd := exec.CommandContext(c,
		"bash",
		"-c",
		`ps ax -o user,pid,%cpu,%mem,vsz,rss,tty,stat,start,time,ppid,command | grep mysqld | grep -v "grep"`,
	)
	procs, err := psCallAndParseWithCmd(cmd)
	if err != nil {
		return nil
	}
	return procs
}

func GetMySQLServerDetails(c context.Context) []*MySQLServerDetail {
	var details []*MySQLServerDetail
	for _, proc := range getMysqldProcess(c) {
		detail := &MySQLServerDetail{
			Process: proc,
			MainPid: proc.Pid,
		}
		mysqldPath := utils.NewBlockParser(bytes.NewBufferString(proc.Command)).NextStringBlock()
		if mysqldPath != "" {
			detail.BinaryFile = mysqldPath
		}

		raw, err := exec.CommandContext(c, mysqldPath, "--verbose", "--help").CombinedOutput()
		if err != nil {
			continue
		}

		s := bufio.NewScanner(bytes.NewBuffer(raw))
		s.Split(bufio.ScanLines)
		for s.Scan() {
			bp := utils.NewBlockParser(bytes.NewBufferString(s.Text()))
			switch bp.NextStringBlock() {
			case "datadir": // -h --datadir=
				path := bp.NextStringBlock()
				r, _ := utils.PathExists(path)
				if r {
					detail.DataDir = path
				}
			case "language":
				path := bp.NextStringBlock()
				r, _ := utils.PathExists(path)
				if r {
					detail.DefaultRootDir = path
				}
			case "mysqld":
				if detail.VersionFull == "" {
					detail.VersionFull = yaklib.Grok(s.Text(), "mysqld[\\s]+Ver[\\s]+%{PROG:full}").Get("full")
				}

				if detail.VersionShort == "" {
					detail.VersionShort = yaklib.Grok(s.Text(), `mysqld[\s]+Ver[\s]+%{COMMONVERSION:short}`).Get("short")
				}
			case "basedir":
				path := bp.NextStringBlock()
				r, _ := utils.PathExists(path)
				if r {
					detail.BaseDir = path
				}
			}
		}

		// config
		for _, cnf := range []string{
			"/etc/my.cnf", "/etc/mysql/my.cnf", "~/.my.cnf",
			"/usr/local/etc/my.cnf", // for macos x
		} {
			if r, _ := utils.PathExists(cnf); r {
				detail.ConfigPath = cnf
				break
			}
		}

		config := yaklib.Grok(proc.Command, "--defaults-file[ =]?%{PROG:config}").Get("config")
		if config != "" {
			detail.ConfigPath = config
		}

		datadir := yaklib.Grok(proc.Command, "((--datadir)|(-h))[ =]?%{PROG:config}").Get("config")
		if datadir != "" {
			detail.DataDir = datadir
		}

		basedir := yaklib.Grok(proc.Command, "((--basedir)|(-b))[ =]?%{PROG:config}").Get("config")
		if basedir != "" {
			detail.BaseDir = basedir
		}

		details = append(details, detail)
	}
	return details
}
