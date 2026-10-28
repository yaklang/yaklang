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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	processes, err := gopsprocess.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return collectProcessInventoryEvents(ctx, processes, readBootID())
}

func collectProcessInventoryEvents(ctx context.Context, processes []*gopsprocess.Process, bootID string) ([]model.Event, error) {
	now := time.Now().UTC()
	snapshots := make([]*processInventorySnapshot, 0, len(processes))
	for _, process := range processes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Collect each visible PID once. Parent details and children are derived
		// from this same inventory instead of re-scanning /proc for every PID.
		snapshot, _ := loadProcessInventorySnapshot(ctx, process, bootID)
		if snapshot != nil {
			// Keep the relationship even when a process disappears before its
			// image can be read; it may still be another process's child.
			snapshots = append(snapshots, snapshot)
		}
	}

	enrichProcessInventoryTree(snapshots)
	if err := enrichMissingProcessInventoryParents(ctx, snapshots); err != nil {
		return nil, err
	}

	events := make([]model.Event, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if snapshot.image == "" && snapshot.command == "" {
			continue
		}
		events = append(events, buildProcessInventoryEventFromSnapshot(now, snapshot))
	}
	return events, ctx.Err()
}

func (systemInventoryProvider) ListNetworkEvents(ctx context.Context) ([]model.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Identity comes from the per-PID snapshot below. Reading socket UIDs here
	// repeats the same /proc/<pid>/status lookup for every socket.
	connections, err := gopsnet.ConnectionsWithoutUidsWithContext(ctx, "all")
	if err != nil {
		return nil, fmt.Errorf("list network connections: %w", err)
	}

	bootID := readBootID()
	processCache := make(map[int32]*processInventorySnapshot)
	now := time.Now().UTC()
	events := make([]model.Event, 0, len(connections))
	for _, connection := range connections {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		event, ok := buildNetworkInventoryEvent(ctx, now, bootID, connection, processCache)
		if !ok {
			continue
		}
		events = append(events, event)
	}
	return events, ctx.Err()
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
	parentDetailsLoaded       bool
	cpuPercent                float64
	memoryPercent             float64
	rssBytes                  int64
	vszBytes                  int64
	threadCount               int
	fdCount                   int
	childrenPIDs              []int
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
	if connection.Pid <= 0 || strings.TrimSpace(bootID) == "" {
		return model.Event{}, false
	}

	snapshot := lookupProcessInventorySnapshot(ctx, connection.Pid, bootID, processCache)
	if snapshot == nil || snapshot.startTimeUnixMillis <= 0 {
		return model.Event{}, false
	}
	processValue := &model.Process{
		PID:                       int(connection.Pid),
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
		BootID:                    bootID,
		StartTimeUnixMillis:       snapshot.startTimeUnixMillis,
		ParentStartTimeUnixMillis: snapshot.parentStartTimeUnixMillis,
	}

	networkValue := &model.Network{
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
	}
	if !model.HasNetworkEndpoint(networkValue) {
		return model.Event{}, false
	}

	return model.Event{
		Type:      model.EventTypeNetworkSocket,
		Source:    networkInventorySource,
		Timestamp: now,
		Tags:      []string{"network", "inventory", "baseline", "state"},
		Process:   processValue,
		Network:   networkValue,
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
	if pid <= 0 || ctx.Err() != nil {
		return nil
	}
	snapshot := lookupNetworkProcessIdentity(ctx, pid, bootID, processCache)
	if snapshot == nil || snapshot.parentDetailsLoaded {
		return snapshot
	}
	if snapshot.parentPID > 0 {
		parent := lookupNetworkProcessIdentity(ctx, int32(snapshot.parentPID), bootID, processCache)
		if parent != nil {
			snapshot.parentName = parent.name
			snapshot.parentImage = parent.image
			snapshot.parentCommand = parent.command
			snapshot.parentStartTimeUnixMillis = parent.startTimeUnixMillis
		}
	}
	snapshot.parentDetailsLoaded = true
	return snapshot
}

func lookupNetworkProcessIdentity(
	ctx context.Context,
	pid int32,
	bootID string,
	processCache map[int32]*processInventorySnapshot,
) *processInventorySnapshot {
	if pid <= 0 || ctx.Err() != nil {
		return nil
	}
	if snapshot, exists := processCache[pid]; exists {
		return snapshot
	}

	// The PID came from the same proc socket inventory. Read its identity there
	// directly: NewProcess's kill(pid, 0) checks the caller's PID namespace rather
	// than a selected HOST_PROC view and does not establish snapshot identity.
	snapshot, ok := loadProcessIdentitySnapshot(ctx, &gopsprocess.Process{Pid: pid}, bootID)
	if !ok {
		processCache[pid] = nil
		return nil
	}
	processCache[pid] = snapshot
	return snapshot
}

// Network observations use identity and parent display fields only. Keeping
// those separate avoids process-wide metrics, FD enumeration, and Children's
// full /proc scan for every distinct network PID.
func loadProcessIdentitySnapshot(ctx context.Context, process *gopsprocess.Process, bootID string) (*processInventorySnapshot, bool) {
	if process == nil || ctx.Err() != nil {
		return nil, false
	}

	pid := int(process.Pid)
	name, err := process.NameWithContext(ctx)
	if err != nil && pid <= 0 {
		return nil, false
	}

	parentPID, _ := process.PpidWithContext(ctx)
	image, _ := process.ExeWithContext(ctx)
	command, _ := process.CmdlineWithContext(ctx)
	username, _ := process.UsernameWithContext(ctx)
	createTime, _ := process.CreateTimeWithContext(ctx)

	uid := ""
	if uids, err := process.UidsWithContext(ctx); err == nil && len(uids) > 0 {
		uid = strconv.FormatUint(uint64(uids[0]), 10)
	}
	gid := ""
	if gids, err := process.GidsWithContext(ctx); err == nil && len(gids) > 0 {
		gid = strconv.FormatUint(uint64(gids[0]), 10)
	}

	if image == "" {
		image = name
	}
	if command == "" {
		command = image
	}
	if ctx.Err() != nil {
		return nil, false
	}

	return &processInventorySnapshot{
		pid:                 pid,
		parentPID:           int(parentPID),
		name:                name,
		image:               image,
		command:             command,
		username:            username,
		uid:                 uid,
		gid:                 gid,
		bootID:              bootID,
		startTimeUnixMillis: createTime,
	}, image != "" || command != ""
}

func loadProcessInventorySnapshot(ctx context.Context, process *gopsprocess.Process, bootID string) (*processInventorySnapshot, bool) {
	snapshot, readable := loadProcessIdentitySnapshot(ctx, process, bootID)
	if snapshot == nil {
		return nil, false
	}
	statuses, _ := process.StatusWithContext(ctx)
	snapshot.status = strings.Join(statuses, ",")
	snapshot.cpuPercent, _ = process.CPUPercentWithContext(ctx)
	memoryPercent, _ := process.MemoryPercentWithContext(ctx)
	snapshot.memoryPercent = float64(memoryPercent)
	threadCount, _ := process.NumThreadsWithContext(ctx)
	snapshot.threadCount = int(threadCount)
	fdCount, _ := process.NumFDsWithContext(ctx)
	snapshot.fdCount = int(fdCount)
	if memoryInfo, err := process.MemoryInfoWithContext(ctx); err == nil && memoryInfo != nil {
		snapshot.rssBytes = int64(memoryInfo.RSS)
		snapshot.vszBytes = int64(memoryInfo.VMS)
	}
	return snapshot, readable && ctx.Err() == nil
}

// A parent can start after the PID enumeration. Only those missing parents
// require a separate lookup, shared by all their children.
func enrichMissingProcessInventoryParents(ctx context.Context, snapshots []*processInventorySnapshot) error {
	byPID := make(map[int]*processInventorySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byPID[snapshot.pid] = snapshot
	}
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return err
		}
		if snapshot.parentPID <= 0 {
			continue
		}
		parent, exists := byPID[snapshot.parentPID]
		if !exists {
			if process, err := gopsprocess.NewProcessWithContext(ctx, int32(snapshot.parentPID)); err == nil {
				name, _ := process.NameWithContext(ctx)
				image, _ := process.ExeWithContext(ctx)
				command, _ := process.CmdlineWithContext(ctx)
				started, _ := process.CreateTimeWithContext(ctx)
				parent = &processInventorySnapshot{name: name, image: image, command: command, startTimeUnixMillis: started}
			}
			byPID[snapshot.parentPID] = parent
		}
		if parent != nil {
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
		}
	}
	return ctx.Err()
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

	collectProcessInventoryObservations(seedCtx, spec, provider, sink)
	collectNetworkInventoryObservations(seedCtx, spec, provider, sink)
	collectHostUserInventoryObservations(seedCtx, spec, provider, sink)
}

func collectProcessInventoryObservations(
	ctx context.Context,
	spec model.DesiredSpec,
	provider inventoryProvider,
	sink chan<- model.Event,
) {
	if !spec.Collectors.Process.Enabled {
		return
	}
	if err := collectProcessCurrentStateObservations(ctx, provider, sink); err != nil {
		log.Warnf("seed hids process inventory failed: %v", err)
	}
}

func collectProcessCurrentStateObservations(
	ctx context.Context,
	provider inventoryProvider,
	sink chan<- model.Event,
) error {
	processEvents, err := provider.ListProcessEvents(ctx)
	if err != nil {
		return fmt.Errorf("list process inventory: %w", err)
	}
	return publishInventoryEvents(ctx, sink, processEvents)
}

func collectNetworkInventoryObservations(
	ctx context.Context,
	spec model.DesiredSpec,
	provider inventoryProvider,
	sink chan<- model.Event,
) {
	if !spec.Collectors.Network.Enabled {
		return
	}
	if err := collectNetworkCurrentStateObservations(ctx, provider, sink); err != nil {
		log.Warnf("seed hids network inventory failed: %v", err)
	}
}

func collectNetworkCurrentStateObservations(
	ctx context.Context,
	provider inventoryProvider,
	sink chan<- model.Event,
) error {
	networkEvents, err := provider.ListNetworkEvents(ctx)
	if err != nil {
		return fmt.Errorf("list network inventory: %w", err)
	}
	return publishInventoryEvents(ctx, sink, networkEvents)
}

func collectHostUserInventoryObservations(
	ctx context.Context,
	spec model.DesiredSpec,
	provider inventoryProvider,
	sink chan<- model.Event,
) {
	if hasAnyEnabledCollector(spec) {
		userEvents, err := provider.ListHostUserEvents(ctx)
		if err != nil {
			log.Warnf("seed hids host user inventory failed: %v", err)
		} else {
			if err := publishInventoryEvents(ctx, sink, userEvents); err != nil {
				log.Warnf("seed hids host user inventory failed: %v", err)
			}
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
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case sink <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
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
