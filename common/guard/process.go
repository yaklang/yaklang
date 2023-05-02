package guard

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/ReneKroon/ttlcache"
	"github.com/google/shlex"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type PsAuxProcessEventType string

const (
	PsAuxProcessEvent_New       PsAuxProcessEventType = "new"
	PsAuxProcessEvent_Disappear PsAuxProcessEventType = "disappear"
)

type PsAuxProcessEventCallback func(name PsAuxProcessEventType, proc *PsProcess)
type PsAuxProcessCallback func([]*PsProcess)

type PsAuxProcessGuardTarget struct {
	guardTargetBase

	eventCallbacks []PsAuxProcessEventCallback
	callbacks      []PsAuxProcessCallback
	cache          *ttlcache.Cache
}

func NewPsAuxProcessGuardTarget(intervalSeconds int, options ...PsAuxProcessGuardOption) (*PsAuxProcessGuardTarget, error) {
	t := &PsAuxProcessGuardTarget{
		guardTargetBase: guardTargetBase{
			intervalSeconds: intervalSeconds,
		},
		cache: ttlcache.NewCache(),
	}
	t.children = t
	for _, option := range options {
		err := option(t)
		if err != nil {
			return nil, utils.Errorf("ps aux guard execute option failed; %s", err)
		}
	}

	if t.eventCallbacks != nil {
		t.cache.SetExpirationCallback(func(pid string, process interface{}) {
			for _, i := range t.eventCallbacks {
				i(PsAuxProcessEvent_Disappear, process.(*PsProcess))
			}
		})
		t.cache.SetNewItemCallback(func(pid string, process interface{}) {
			for _, i := range t.eventCallbacks {
				i(PsAuxProcessEvent_New, process.(*PsProcess))
			}
		})
	}

	return t, nil
}

type PsAuxProcessGuardOption func(t *PsAuxProcessGuardTarget) error

func SetPsAuxProcessCallback(f PsAuxProcessCallback) PsAuxProcessGuardOption {
	return func(t *PsAuxProcessGuardTarget) error {
		t.callbacks = append(t.callbacks, f)
		return nil
	}
}

func SetPsAuxProcessEventCallback(f PsAuxProcessEventCallback) PsAuxProcessGuardOption {
	return func(t *PsAuxProcessGuardTarget) error {
		t.eventCallbacks = append(t.eventCallbacks, f)
		return nil
	}
}

func (p *PsAuxProcessGuardTarget) do() {
	ps, err := CallPsAux(utils.TimeoutContext(time.Duration(p.intervalSeconds) * time.Second))
	if err != nil {
		log.Errorf("found process failed: %s", err)
		return
	}

	for _, i := range p.callbacks {
		i(ps)
	}

	for _, proc := range ps {
		p.cache.SetWithTTL(fmt.Sprint(proc.Pid), proc, time.Duration(2*p.intervalSeconds)*time.Second)
	}

	return
}

type PsProcess struct {
	Raw             []byte
	User            string
	Pid             int
	CPUPercent      float64
	MEMPercent      float64
	Vsz             int64
	Rss             int64
	Tty             string
	Stat            string
	Started         string
	DurationTimeRaw string
	Command         string
	ProcessName     string
	ParentPid       int32
	ChildrenPid     []int32
}

func psCallAndParseWithCmd(cmd *exec.Cmd) ([]*PsProcess, error) {
	switch runtime.GOOS {
	case "windows":
		return nil, utils.Errorf("windows is not supported")
	}

	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, utils.Errorf("exec ps aux failed: %s", err)
	}
	var currentPid = -1
	if cmd.ProcessState != nil {
		currentPid = cmd.ProcessState.Pid()
	}

	var (
		procs []*PsProcess
	)

	s := bufio.NewScanner(bytes.NewBuffer(raw))
	s.Split(bufio.ScanLines)
	s.Scan() // drop header

	var (
		parentR  = make(map[int32][]int32)
		pidTable = make(map[int32]*PsProcess)
	)
	for s.Scan() {
		p, err := readPsLine(s.Bytes())
		if err != nil {
			//log.Errorf("parse ps line [%v] failed: %s", string(s.Bytes()), err)
			continue
		}

		if p.Pid == currentPid {
			continue
		}

		parentR[p.ParentPid] = append(parentR[p.ParentPid], int32(p.Pid))
		pidTable[int32(p.Pid)] = p

		procs = append(procs, p)
	}

	for ppid, cpids := range parentR {
		raw, ok := pidTable[ppid]
		if !ok {
			continue
		}
		raw.ChildrenPid = cpids
	}
	return procs, nil
}

func CallPsAux(ctx context.Context) ([]*PsProcess, error) {
	switch runtime.GOOS {
	case "windows":
		return nil, utils.Errorf("windows is not supported")
	}

	cmd := exec.CommandContext(ctx,
		"ps", "ax", "-o",
		"user,pid,%cpu,%mem,vsz,rss,tty,stat,start,time,ppid,command",
	)
	return psCallAndParseWithCmd(cmd)
}

func readPsLine(line []byte) (*PsProcess, error) {
	var (
		buf io.Reader = bytes.NewBuffer(line)
	)
	p := &PsProcess{}

	blockScanner := bufio.NewScanner(buf)
	blockScanner.Split(bufio.ScanWords)
	nextBlock := func() []byte {
		blockScanner.Scan()
		return blockScanner.Bytes()
	}

	p.User = string(nextBlock())
	p.Raw = line

	pidRaw := nextBlock()
	pid, err := strconv.ParseInt(string(pidRaw), 10, 64)
	if err != nil {
		return nil, utils.Errorf("parse pid failed: %s", pidRaw)
	}
	p.Pid = int(pid)

	p.CPUPercent, _ = strconv.ParseFloat(string(nextBlock()), 64)
	p.MEMPercent, _ = strconv.ParseFloat(string(nextBlock()), 64)
	p.Vsz, _ = strconv.ParseInt(string(nextBlock()), 10, 64)
	p.Rss, _ = strconv.ParseInt(string(nextBlock()), 10, 64)
	p.Tty = string(nextBlock())
	p.Stat = string(nextBlock())
	p.Started = string(nextBlock())
	p.DurationTimeRaw = string(nextBlock())

	ppidRaw, _ := strconv.ParseInt(string(nextBlock()), 10, 32)
	p.ParentPid = int32(ppidRaw)

	var words []string
	for blockScanner.Scan() {
		words = append(words, blockScanner.Text())
	}
	p.Command = strings.Join(words, " ")

	raws, _ := shlex.Split(p.Command)
	if len(raws) > 0 {
		p.ProcessName = raws[0]
	}

	return p, nil
}
