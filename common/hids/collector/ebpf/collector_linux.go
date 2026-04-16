//go:build hids && linux

package ebpf

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/log"
)

type Collector struct {
	name string

	mu       sync.Mutex
	events   *ebpf.Map
	programs []*ebpf.Program
	links    []link.Link
	reader   *ringbuf.Reader
	state    ebpfCollectorState
}

type tracepointProgramSpec struct {
	group    string
	name     string
	optional bool
	spec     *ebpf.ProgramSpec
}

var (
	memlockOnce sync.Once
	memlockErr  error
)

func NewProcess() hidscollector.Instance {
	return &Collector{
		name:  "ebpf.process",
		state: newEBPFCollectorState("ebpf.process"),
	}
}

func NewNetwork() hidscollector.Instance {
	return &Collector{
		name:  "ebpf.network",
		state: newEBPFCollectorState("ebpf.network"),
	}
}

func (c *Collector) Name() string {
	return c.name
}

func (c *Collector) Start(ctx context.Context, sink chan<- model.Event) error {
	if err := ensureMemlockReleased(); err != nil {
		return fmt.Errorf("prepare ebpf collector runtime: %w", err)
	}

	events, reader, programs, links, attachedTracepoints, skippedTracepoints, err := c.open()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.events = events
	c.reader = reader
	c.programs = programs
	c.links = links
	c.mu.Unlock()
	c.state.setRunning(attachedTracepoints, skippedTracepoints)

	go c.run(ctx, sink)
	return nil
}

func (c *Collector) Close() error {
	c.mu.Lock()
	events := c.events
	reader := c.reader
	programs := c.programs
	links := c.links
	c.events = nil
	c.reader = nil
	c.programs = nil
	c.links = nil
	c.mu.Unlock()
	c.state.setStopped()

	var closeErrs []error
	if reader != nil {
		closeErrs = append(closeErrs, reader.Close())
	}
	for _, attached := range links {
		if attached != nil {
			closeErrs = append(closeErrs, attached.Close())
		}
	}
	for _, program := range programs {
		if program != nil {
			closeErrs = append(closeErrs, program.Close())
		}
	}
	if events != nil {
		closeErrs = append(closeErrs, events.Close())
	}
	return errors.Join(closeErrs...)
}

func (c *Collector) open() (*ebpf.Map, *ringbuf.Reader, []*ebpf.Program, []link.Link, []string, []string, error) {
	events, err := ebpf.NewMap(newRingbufMapSpec(c.name))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("create ringbuf map: %w", err)
	}

	reader, err := ringbuf.NewReader(events)
	if err != nil {
		_ = events.Close()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("create ringbuf reader: %w", err)
	}

	var programs []*ebpf.Program
	var links []link.Link
	var attachedTracepoints []string
	var skippedTracepoints []string
	cleanup := func() {
		for _, attached := range links {
			if attached != nil {
				_ = attached.Close()
			}
		}
		for _, program := range programs {
			if program != nil {
				_ = program.Close()
			}
		}
		_ = reader.Close()
		_ = events.Close()
	}

	requiredAttached := 0
	for _, tracepoint := range c.tracepointSpecs(events.FD()) {
		program, err := ebpf.NewProgram(tracepoint.spec)
		if err != nil {
			cleanup()
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("load %s/%s program: %w", tracepoint.group, tracepoint.name, err)
		}
		programs = append(programs, program)

		attached, err := link.Tracepoint(tracepoint.group, tracepoint.name, program, nil)
		if err != nil {
			if tracepoint.optional && errors.Is(err, fs.ErrNotExist) {
				log.Warnf("skip optional ebpf tracepoint: collector=%s tracepoint=%s/%s err=%v", c.name, tracepoint.group, tracepoint.name, err)
				skippedTracepoints = append(skippedTracepoints, tracepoint.group+"/"+tracepoint.name)
				continue
			}
			cleanup()
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("attach %s/%s tracepoint: %w", tracepoint.group, tracepoint.name, err)
		}
		requiredAttached++
		links = append(links, attached)
		attachedTracepoints = append(attachedTracepoints, tracepoint.group+"/"+tracepoint.name)
	}

	if requiredAttached == 0 {
		cleanup()
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("collector %s did not attach any tracepoint", c.name)
	}

	return events, reader, programs, links, attachedTracepoints, skippedTracepoints, nil
}

func (c *Collector) run(ctx context.Context, sink chan<- model.Event) {
	c.mu.Lock()
	reader := c.reader
	c.mu.Unlock()
	if reader == nil {
		return
	}

	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) || ctx.Err() != nil {
				return
			}
			c.state.observeReadError(err)
			log.Warnf("read ebpf collector event failed: collector=%s err=%v", c.name, err)
			continue
		}
		c.state.observeReceived()

		event, ok, err := c.decodeRecord(record.RawSample)
		if err != nil {
			c.state.observeDecodeError(err)
			log.Warnf("decode ebpf collector event failed: collector=%s err=%v", c.name, err)
			continue
		}
		if !ok {
			c.state.observeIgnored()
			continue
		}

		select {
		case sink <- event:
			c.state.observeEmitted(event.Timestamp)
		case <-ctx.Done():
			return
		default:
			c.state.observeDropped()
		}
	}
}

func (c *Collector) HealthSnapshot() hidscollector.HealthSnapshot {
	return c.state.snapshot()
}

func (c *Collector) decodeRecord(raw []byte) (model.Event, bool, error) {
	switch decodeRecordKind(raw) {
	case recordKindProcessExec:
		record, err := parseProcessRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return processRecordToEvent(c.name, record), true, nil
	case recordKindProcessExit:
		record, err := parseProcessRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return processExitRecordToEvent(c.name, record), true, nil
	case recordKindNetworkConnect:
		record, err := parseNetworkRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return networkRecordToEvent(c.name, record), true, nil
	case recordKindNetworkClose:
		record, err := parseNetworkRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return networkCloseRecordToEvent(c.name, record), true, nil
	case recordKindNetworkAccept:
		record, err := parseNetworkRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return networkAcceptRecordToEvent(c.name, record), true, nil
	case recordKindNetworkState:
		record, err := parseNetworkStateRecord(raw)
		if err != nil {
			return model.Event{}, false, err
		}
		return networkStateRecordToEvent(c.name, record), true, nil
	default:
		return model.Event{}, false, nil
	}
}

func (c *Collector) tracepointSpecs(eventsFD int) []tracepointProgramSpec {
	switch c.name {
	case "ebpf.process":
		return []tracepointProgramSpec{
			{
				group: "syscalls",
				name:  "sys_enter_execve",
				spec:  buildProcessExecProgramSpec("hids_execve", processExecveFilenameArgOffset, eventsFD),
			},
			{
				group:    "syscalls",
				name:     "sys_enter_execveat",
				optional: true,
				spec:     buildProcessExecProgramSpec("hids_execveat", processExecveAtFilenameArgOffset, eventsFD),
			},
			{
				group: "sched",
				name:  "sched_process_exit",
				spec:  buildProcessExitProgramSpec("hids_exit", eventsFD),
			},
		}
	case "ebpf.network":
		return []tracepointProgramSpec{
			{
				group:    "syscalls",
				name:     "sys_exit_accept",
				optional: true,
				spec:     buildNetworkAcceptProgramSpec("hids_accept", eventsFD),
			},
			{
				group:    "syscalls",
				name:     "sys_exit_accept4",
				optional: true,
				spec:     buildNetworkAcceptProgramSpec("hids_accept4", eventsFD),
			},
			{
				group: "syscalls",
				name:  "sys_enter_connect",
				spec:  buildNetworkConnectProgramSpec("hids_connect", eventsFD),
			},
			{
				group: "syscalls",
				name:  "sys_enter_close",
				spec:  buildNetworkCloseProgramSpec("hids_close", eventsFD),
			},
			{
				group:    "sock",
				name:     "inet_sock_set_state",
				optional: true,
				spec:     buildNetworkStateProgramSpec("hids_state", eventsFD),
			},
		}
	default:
		return nil
	}
}

func ensureMemlockReleased() error {
	memlockOnce.Do(func() {
		memlockErr = rlimit.RemoveMemlock()
	})
	return memlockErr
}
