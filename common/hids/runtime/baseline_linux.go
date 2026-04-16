//go:build hids && linux

package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	gopsprocess "github.com/shirou/gopsutil/v4/process"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/log"
)

const (
	processInventorySource = "inventory.process"
	networkInventorySource = "inventory.network"
)

type inventoryProvider interface {
	ListProcessEvents(context.Context) ([]model.Event, error)
	ListNetworkEvents(context.Context) ([]model.Event, error)
}

type systemInventoryProvider struct{}

func newSystemInventoryProvider() inventoryProvider {
	return systemInventoryProvider{}
}

func (systemInventoryProvider) ListProcessEvents(ctx context.Context) ([]model.Event, error) {
	processes, err := gopsprocess.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}

	now := time.Now().UTC()
	events := make([]model.Event, 0, len(processes))
	for _, process := range processes {
		event, ok := buildProcessInventoryEvent(now, process)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func (systemInventoryProvider) ListNetworkEvents(ctx context.Context) ([]model.Event, error) {
	connections, err := gopsnet.ConnectionsWithContext(ctx, "all")
	if err != nil {
		return nil, fmt.Errorf("list network connections: %w", err)
	}

	processCache := make(map[int32]*processInventorySnapshot)
	now := time.Now().UTC()
	events := make([]model.Event, 0, len(connections))
	for _, connection := range connections {
		event, ok := buildNetworkInventoryEvent(ctx, now, connection, processCache)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

type processInventorySnapshot struct {
	pid        int
	parentPID  int
	name       string
	image      string
	command    string
	parentName string
	username   string
	statuses   []string
}

func buildProcessInventoryEvent(
	now time.Time,
	process *gopsprocess.Process,
) (model.Event, bool) {
	if process == nil {
		return model.Event{}, false
	}

	snapshot, ok := loadProcessInventorySnapshot(process)
	if !ok {
		return model.Event{}, false
	}

	data := map[string]any{
		"inventory": true,
	}
	if snapshot.username != "" {
		data["username"] = snapshot.username
	}
	if len(snapshot.statuses) > 0 {
		data["status"] = strings.Join(snapshot.statuses, ",")
	}

	return model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    processInventorySource,
		Timestamp: now,
		Tags:      []string{"process", "inventory", "baseline"},
		Process: &model.Process{
			PID:        snapshot.pid,
			ParentPID:  snapshot.parentPID,
			Name:       snapshot.name,
			Username:   snapshot.username,
			Image:      snapshot.image,
			Command:    snapshot.command,
			ParentName: snapshot.parentName,
		},
		Data: data,
	}, true
}

func buildNetworkInventoryEvent(
	ctx context.Context,
	now time.Time,
	connection gopsnet.ConnectionStat,
	processCache map[int32]*processInventorySnapshot,
) (model.Event, bool) {
	if connection.Raddr.IP == "" && connection.Raddr.Port == 0 {
		return model.Event{}, false
	}

	protocol := networkProtocol(connection)
	if protocol == "" {
		return model.Event{}, false
	}

	snapshot := lookupProcessInventorySnapshot(ctx, connection.Pid, processCache)
	processValue := &model.Process{
		PID: int(connection.Pid),
	}
	if snapshot != nil {
		processValue.ParentPID = snapshot.parentPID
		processValue.Name = snapshot.name
		processValue.Username = snapshot.username
		processValue.Image = snapshot.image
		processValue.Command = snapshot.command
		processValue.ParentName = snapshot.parentName
	}

	data := map[string]any{
		"inventory":        true,
		"status":           strings.TrimSpace(connection.Status),
		"family":           int(connection.Family),
		"socket_type":      int(connection.Type),
		"local_address":    connection.Laddr.IP,
		"remote_address":   connection.Raddr.IP,
		"local_port":       int(connection.Laddr.Port),
		"remote_port":      int(connection.Raddr.Port),
		"connection_state": strings.TrimSpace(connection.Status),
	}

	return model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    networkInventorySource,
		Timestamp: now,
		Tags:      []string{"network", "inventory", "baseline"},
		Process:   processValue,
		Network: &model.Network{
			Protocol:        protocol,
			SourceAddress:   connection.Laddr.IP,
			DestAddress:     connection.Raddr.IP,
			SourcePort:      int(connection.Laddr.Port),
			DestPort:        int(connection.Raddr.Port),
			ConnectionState: strings.TrimSpace(connection.Status),
		},
		Data: data,
	}, true
}

func lookupProcessInventorySnapshot(
	ctx context.Context,
	pid int32,
	processCache map[int32]*processInventorySnapshot,
) *processInventorySnapshot {
	if pid <= 0 {
		return nil
	}
	if snapshot, ok := processCache[pid]; ok {
		return snapshot
	}

	process, err := gopsprocess.NewProcessWithContext(ctx, pid)
	if err != nil {
		processCache[pid] = nil
		return nil
	}
	snapshot, ok := loadProcessInventorySnapshot(process)
	if !ok {
		processCache[pid] = nil
		return nil
	}
	processCache[pid] = snapshot
	return snapshot
}

func loadProcessInventorySnapshot(process *gopsprocess.Process) (*processInventorySnapshot, bool) {
	if process == nil {
		return nil, false
	}

	pid := int(process.Pid)
	name, err := process.Name()
	if err != nil && pid <= 0 {
		return nil, false
	}

	parentPID, _ := process.Ppid()
	image, _ := process.Exe()
	command, _ := process.Cmdline()
	username, _ := process.Username()
	statuses, _ := process.Status()

	parentName := ""
	if parentPID > 0 {
		if parent, err := gopsprocess.NewProcess(parentPID); err == nil {
			parentName, _ = parent.Name()
		}
	}

	if image == "" {
		image = name
	}
	if command == "" {
		command = image
	}
	if image == "" && command == "" {
		return nil, false
	}

	return &processInventorySnapshot{
		pid:        pid,
		parentPID:  int(parentPID),
		name:       name,
		image:      image,
		command:    command,
		parentName: parentName,
		username:   username,
		statuses:   statuses,
	}, true
}

func networkProtocol(connection gopsnet.ConnectionStat) string {
	switch connection.Type {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	default:
		return ""
	}
}

func emitInventoryObservations(
	ctx context.Context,
	spec model.DesiredSpec,
	provider inventoryProvider,
	sink chan<- model.Event,
) {
	if provider == nil || sink == nil {
		return
	}

	seedCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	if spec.Collectors.Process.Enabled {
		processEvents, err := provider.ListProcessEvents(seedCtx)
		if err != nil {
			log.Warnf("seed hids process inventory failed: %v", err)
		} else {
			publishInventoryEvents(seedCtx, sink, processEvents)
		}
	}

	if spec.Collectors.Network.Enabled {
		networkEvents, err := provider.ListNetworkEvents(seedCtx)
		if err != nil {
			log.Warnf("seed hids network inventory failed: %v", err)
		} else {
			publishInventoryEvents(seedCtx, sink, networkEvents)
		}
	}
}

func publishInventoryEvents(
	ctx context.Context,
	sink chan<- model.Event,
	events []model.Event,
) {
	for _, event := range events {
		select {
		case sink <- event:
		case <-ctx.Done():
			return
		}
	}
}
