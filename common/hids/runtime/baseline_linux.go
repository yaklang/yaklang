//go:build hids && linux

package runtime

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	gopsprocess "github.com/shirou/gopsutil/v4/process"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/log"
)

const (
	processInventorySource   = "inventory.process"
	networkInventorySource   = "inventory.network"
	hostUsersInventorySource = "inventory.users"
)

type inventoryProvider interface {
	ListProcessEvents(context.Context) ([]model.Event, error)
	ListNetworkEvents(context.Context) ([]model.Event, error)
	ListHostUserEvents(context.Context) ([]model.Event, error)
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

	bootID := readBootID()
	now := time.Now().UTC()
	snapshots := make([]*processInventorySnapshot, 0, len(processes))
	for _, process := range processes {
		snapshot, ok := loadProcessInventorySnapshot(process, bootID)
		if !ok {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	enrichProcessInventoryTree(snapshots)

	events := make([]model.Event, 0, len(snapshots))
	for _, snapshot := range snapshots {
		events = append(events, buildProcessInventoryEventFromSnapshot(now, snapshot))
	}
	return events, nil
}

func (systemInventoryProvider) ListNetworkEvents(ctx context.Context) ([]model.Event, error) {
	connections, err := gopsnet.ConnectionsWithContext(ctx, "all")
	if err != nil {
		return nil, fmt.Errorf("list network connections: %w", err)
	}

	bootID := readBootID()
	processCache := make(map[int32]*processInventorySnapshot)
	now := time.Now().UTC()
	events := make([]model.Event, 0, len(connections))
	for _, connection := range connections {
		event, ok := buildNetworkInventoryEvent(ctx, now, bootID, connection, processCache)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func (systemInventoryProvider) ListHostUserEvents(_ context.Context) ([]model.Event, error) {
	users, err := loadHostUsers()
	if err != nil {
		return nil, fmt.Errorf("list host users: %w", err)
	}
	now := time.Now().UTC()
	return []model.Event{{
		Type:      model.EventTypeHostUsers,
		Source:    hostUsersInventorySource,
		Timestamp: now,
		Tags:      []string{"host", "user", "inventory", "baseline"},
		Users:     users,
		Data: map[string]any{
			"inventory":  true,
			"user_count": len(users),
		},
	}}, nil
}

type processInventorySnapshot struct {
	pid                       int
	parentPID                 int
	name                      string
	image                     string
	command                   string
	parentName                string
	parentImage               string
	parentCommand             string
	username                  string
	uid                       string
	gid                       string
	status                    string
	bootID                    string
	startTimeUnixMillis       int64
	parentStartTimeUnixMillis int64
	cpuPercent                float64
	memoryPercent             float64
	rssBytes                  int64
	vszBytes                  int64
	threadCount               int
	fdCount                   int
	childrenPIDs              []int
}

func buildProcessInventoryEvent(
	now time.Time,
	bootID string,
	process *gopsprocess.Process,
) (model.Event, bool) {
	if process == nil {
		return model.Event{}, false
	}

	snapshot, ok := loadProcessInventorySnapshot(process, bootID)
	if !ok {
		return model.Event{}, false
	}

	return buildProcessInventoryEventFromSnapshot(now, snapshot), true
}

func buildProcessInventoryEventFromSnapshot(
	now time.Time,
	snapshot *processInventorySnapshot,
) model.Event {
	return model.Event{
		Type:      model.EventTypeProcessState,
		Source:    processInventorySource,
		Timestamp: now,
		Tags:      []string{"process", "inventory", "baseline", "state"},
		Process: &model.Process{
			PID:                       snapshot.pid,
			ParentPID:                 snapshot.parentPID,
			Name:                      snapshot.name,
			Username:                  snapshot.username,
			UID:                       snapshot.uid,
			GID:                       snapshot.gid,
			Image:                     snapshot.image,
			Command:                   snapshot.command,
			ParentName:                snapshot.parentName,
			ParentImage:               snapshot.parentImage,
			ParentCommand:             snapshot.parentCommand,
			BootID:                    snapshot.bootID,
			StartTimeUnixMillis:       snapshot.startTimeUnixMillis,
			ParentStartTimeUnixMillis: snapshot.parentStartTimeUnixMillis,
			State:                     snapshot.status,
			CPUPercent:                snapshot.cpuPercent,
			MemoryPercent:             snapshot.memoryPercent,
			RSSBytes:                  snapshot.rssBytes,
			VSZBytes:                  snapshot.vszBytes,
			ThreadCount:               snapshot.threadCount,
			FDCount:                   snapshot.fdCount,
			ChildrenPIDs:              cloneIntSlice(snapshot.childrenPIDs),
		},
		Data: processInventoryEventData(snapshot),
	}
}

func buildNetworkInventoryEvent(
	ctx context.Context,
	now time.Time,
	bootID string,
	connection gopsnet.ConnectionStat,
	processCache map[int32]*processInventorySnapshot,
) (model.Event, bool) {
	protocol := networkProtocol(connection)
	if protocol == "" {
		return model.Event{}, false
	}

	snapshot := lookupProcessInventorySnapshot(ctx, connection.Pid, bootID, processCache)
	processValue := &model.Process{
		PID:    int(connection.Pid),
		BootID: bootID,
	}
	if snapshot != nil {
		processValue.ParentPID = snapshot.parentPID
		processValue.Name = snapshot.name
		processValue.Username = snapshot.username
		processValue.UID = snapshot.uid
		processValue.GID = snapshot.gid
		processValue.Image = snapshot.image
		processValue.Command = snapshot.command
		processValue.ParentName = snapshot.parentName
		processValue.ParentImage = snapshot.parentImage
		processValue.ParentCommand = snapshot.parentCommand
		processValue.StartTimeUnixMillis = snapshot.startTimeUnixMillis
		processValue.ParentStartTimeUnixMillis = snapshot.parentStartTimeUnixMillis
	}

	return model.Event{
		Type:      model.EventTypeNetworkSocket,
		Source:    networkInventorySource,
		Timestamp: now,
		Tags:      []string{"network", "inventory", "baseline", "state"},
		Process:   processValue,
		Network: &model.Network{
			Protocol:        protocol,
			SourceAddress:   connection.Laddr.IP,
			DestAddress:     connection.Raddr.IP,
			SourcePort:      int(connection.Laddr.Port),
			DestPort:        int(connection.Raddr.Port),
			ConnectionState: strings.TrimSpace(connection.Status),
			Direction:       inferConnectionDirection(connection),
			FD:              int(connection.Fd),
			Family:          connectionFamily(connection.Family),
			SocketType:      connectionSocketType(connection.Type),
		},
		Data: map[string]any{
			"inventory": true,
		},
	}, true
}

func lookupProcessInventorySnapshot(
	ctx context.Context,
	pid int32,
	bootID string,
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
	snapshot, ok := loadProcessInventorySnapshot(process, bootID)
	if !ok {
		processCache[pid] = nil
		return nil
	}
	processCache[pid] = snapshot
	return snapshot
}

func loadProcessInventorySnapshot(process *gopsprocess.Process, bootID string) (*processInventorySnapshot, bool) {
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
	createTime, _ := process.CreateTime()
	cpuPercent, _ := process.CPUPercent()
	memoryPercent, _ := process.MemoryPercent()
	threadCount, _ := process.NumThreads()
	fdCount, _ := process.NumFDs()

	var rssBytes int64
	var vszBytes int64
	if memoryInfo, err := process.MemoryInfo(); err == nil && memoryInfo != nil {
		rssBytes = int64(memoryInfo.RSS)
		vszBytes = int64(memoryInfo.VMS)
	}

	uid := ""
	if uids, err := process.Uids(); err == nil && len(uids) > 0 {
		uid = strconv.FormatUint(uint64(uids[0]), 10)
	}
	gid := ""
	if gids, err := process.Gids(); err == nil && len(gids) > 0 {
		gid = strconv.FormatUint(uint64(gids[0]), 10)
	}

	parentName := ""
	parentImage := ""
	parentCommand := ""
	parentStartTimeUnixMillis := int64(0)
	if parentPID > 0 {
		if parent, err := gopsprocess.NewProcess(parentPID); err == nil {
			parentName, _ = parent.Name()
			parentImage, _ = parent.Exe()
			parentCommand, _ = parent.Cmdline()
			parentStartTimeUnixMillis, _ = parent.CreateTime()
		}
	}

	childrenPIDs := make([]int, 0)
	if children, err := process.Children(); err == nil {
		for _, child := range children {
			childrenPIDs = append(childrenPIDs, int(child.Pid))
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

	status := ""
	if len(statuses) > 0 {
		status = strings.Join(statuses, ",")
	}

	return &processInventorySnapshot{
		pid:                       pid,
		parentPID:                 int(parentPID),
		name:                      name,
		image:                     image,
		command:                   command,
		parentName:                parentName,
		parentImage:               parentImage,
		parentCommand:             parentCommand,
		username:                  username,
		uid:                       uid,
		gid:                       gid,
		status:                    status,
		bootID:                    bootID,
		startTimeUnixMillis:       createTime,
		parentStartTimeUnixMillis: parentStartTimeUnixMillis,
		cpuPercent:                cpuPercent,
		memoryPercent:             float64(memoryPercent),
		rssBytes:                  rssBytes,
		vszBytes:                  vszBytes,
		threadCount:               int(threadCount),
		fdCount:                   int(fdCount),
		childrenPIDs:              childrenPIDs,
	}, true
}

func enrichProcessInventoryTree(snapshots []*processInventorySnapshot) {
	snapshotByPID := make(map[int]*processInventorySnapshot, len(snapshots))
	childrenByParentPID := make(map[int][]int)

	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.pid <= 0 {
			continue
		}
		snapshotByPID[snapshot.pid] = snapshot
	}

	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.parentPID <= 0 {
			continue
		}
		parent := snapshotByPID[snapshot.parentPID]
		if parent == nil {
			continue
		}
		if snapshot.parentName == "" {
			snapshot.parentName = parent.name
		}
		if snapshot.parentImage == "" {
			snapshot.parentImage = parent.image
		}
		if snapshot.parentCommand == "" {
			snapshot.parentCommand = parent.command
		}
		if snapshot.parentStartTimeUnixMillis == 0 {
			snapshot.parentStartTimeUnixMillis = parent.startTimeUnixMillis
		}
		childrenByParentPID[parent.pid] = append(childrenByParentPID[parent.pid], snapshot.pid)
	}

	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		snapshot.childrenPIDs = mergeSortedUniqueInts(snapshot.childrenPIDs, childrenByParentPID[snapshot.pid])
	}
}

func processInventoryEventData(snapshot *processInventorySnapshot) map[string]any {
	data := map[string]any{
		"inventory": true,
	}
	if snapshot == nil || snapshot.parentPID <= 0 {
		return data
	}

	parent := map[string]any{
		"pid": snapshot.parentPID,
	}
	if snapshot.parentName != "" {
		parent["name"] = snapshot.parentName
	}
	if snapshot.parentImage != "" {
		parent["image"] = snapshot.parentImage
	}
	if snapshot.parentCommand != "" {
		parent["command"] = snapshot.parentCommand
	}
	if snapshot.parentStartTimeUnixMillis > 0 {
		parent["start_time_unix_ms"] = snapshot.parentStartTimeUnixMillis
	}
	data["parent_process"] = parent
	return data
}

func mergeSortedUniqueInts(left []int, right []int) []int {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(left)+len(right))
	for _, value := range left {
		if value <= 0 {
			continue
		}
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if value <= 0 {
			continue
		}
		seen[value] = struct{}{}
	}
	values := make([]int, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Ints(values)
	return values
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

func connectionFamily(value uint32) string {
	switch value {
	case 2:
		return "inet"
	case 10, 30:
		return "inet6"
	case 1:
		return "unix"
	default:
		return strconv.FormatUint(uint64(value), 10)
	}
}

func connectionSocketType(value uint32) string {
	switch value {
	case 1:
		return "stream"
	case 2:
		return "dgram"
	case 3:
		return "raw"
	default:
		return strconv.FormatUint(uint64(value), 10)
	}
}

func inferConnectionDirection(connection gopsnet.ConnectionStat) string {
	if connection.Raddr.IP == "" && connection.Raddr.Port == 0 {
		return "listen"
	}
	if connection.Laddr.IP == "127.0.0.1" || connection.Laddr.IP == "::1" ||
		connection.Raddr.IP == "127.0.0.1" || connection.Raddr.IP == "::1" {
		return "local"
	}
	if connection.Laddr.Port > 0 && connection.Raddr.Port > 0 && connection.Laddr.Port <= 1024 && connection.Raddr.Port > connection.Laddr.Port {
		return "inbound"
	}
	return "outbound"
}

func readBootID() string {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
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

	if hasAnyEnabledCollector(spec) {
		userEvents, err := provider.ListHostUserEvents(seedCtx)
		if err != nil {
			log.Warnf("seed hids host user inventory failed: %v", err)
		} else {
			publishInventoryEvents(seedCtx, sink, userEvents)
		}
	}
}

func hasAnyEnabledCollector(spec model.DesiredSpec) bool {
	return spec.Collectors.Process.Enabled ||
		spec.Collectors.Network.Enabled ||
		spec.Collectors.File.Enabled ||
		spec.Collectors.Audit.Enabled
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

type groupInventory struct {
	name    string
	members map[string]struct{}
}

func loadHostUsers() ([]model.HostUser, error) {
	groupByGID, memberships, err := loadGroupInventory()
	if err != nil {
		return nil, err
	}

	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	users := make([]model.HostUser, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		username := parts[0]
		uid := parts[2]
		gid := parts[3]
		home := parts[5]
		shell := parts[6]
		groups := collectUserGroups(username, gid, groupByGID, memberships)
		users = append(users, model.HostUser{
			Username:     username,
			UID:          uid,
			GID:          gid,
			Home:         home,
			Shell:        shell,
			Groups:       groups,
			System:       isSystemUID(uid),
			LoginEnabled: isLoginEnabledShell(shell),
			Privileged:   uid == "0" || hasPrivilegedGroup(groups),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(users, func(left, right int) bool {
		return users[left].Username < users[right].Username
	})
	return users, nil
}

func loadGroupInventory() (map[string]string, map[string]map[string]struct{}, error) {
	file, err := os.Open("/etc/group")
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	groupByGID := make(map[string]string)
	memberships := make(map[string]map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		gid := parts[2]
		groupByGID[gid] = name
		for _, member := range strings.Split(parts[3], ",") {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if memberships[member] == nil {
				memberships[member] = make(map[string]struct{})
			}
			memberships[member][name] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return groupByGID, memberships, nil
}

func collectUserGroups(
	username string,
	primaryGID string,
	groupByGID map[string]string,
	memberships map[string]map[string]struct{},
) []string {
	groupSet := make(map[string]struct{})
	if groupName := strings.TrimSpace(groupByGID[primaryGID]); groupName != "" {
		groupSet[groupName] = struct{}{}
	}
	for groupName := range memberships[username] {
		groupSet[groupName] = struct{}{}
	}
	groups := make([]string, 0, len(groupSet))
	for groupName := range groupSet {
		groups = append(groups, groupName)
	}
	sort.Strings(groups)
	return groups
}

func isSystemUID(uid string) bool {
	value, err := strconv.Atoi(strings.TrimSpace(uid))
	if err != nil {
		return false
	}
	return value > 0 && value < 1000
}

func isLoginEnabledShell(shell string) bool {
	shell = strings.TrimSpace(shell)
	if shell == "" {
		return false
	}
	return !strings.Contains(shell, "nologin") &&
		!strings.Contains(shell, "false")
}

func hasPrivilegedGroup(groups []string) bool {
	for _, group := range groups {
		switch strings.ToLower(strings.TrimSpace(group)) {
		case "root", "wheel", "sudo", "admin", "adm":
			return true
		}
	}
	return false
}
