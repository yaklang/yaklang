//go:build hids && linux

package runtime

import (
	"context"
	"testing"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func TestEnrichProcessInventoryTreeLinksParentsAndChildren(t *testing.T) {
	t.Parallel()

	parent := &processInventorySnapshot{
		pid:                 100,
		name:                "systemd",
		image:               "/usr/lib/systemd/systemd",
		command:             "/usr/lib/systemd/systemd",
		startTimeUnixMillis: 1000,
	}
	child := &processInventorySnapshot{
		pid:       200,
		parentPID: 100,
		name:      "sshd",
		image:     "/usr/sbin/sshd",
		command:   "/usr/sbin/sshd -D",
	}
	anotherChild := &processInventorySnapshot{
		pid:       150,
		parentPID: 100,
		name:      "cron",
		image:     "/usr/sbin/cron",
		command:   "/usr/sbin/cron -f",
	}

	enrichProcessInventoryTree([]*processInventorySnapshot{child, parent, anotherChild})

	if child.parentName != "systemd" {
		t.Fatalf("expected child parent name from inventory snapshot, got %q", child.parentName)
	}
	if child.parentImage != "/usr/lib/systemd/systemd" {
		t.Fatalf("expected child parent image from inventory snapshot, got %q", child.parentImage)
	}
	if child.parentCommand != "/usr/lib/systemd/systemd" {
		t.Fatalf("expected child parent command from inventory snapshot, got %q", child.parentCommand)
	}
	if child.parentStartTimeUnixMillis != 1000 {
		t.Fatalf("expected child parent start time from inventory snapshot, got %d", child.parentStartTimeUnixMillis)
	}
	if got := parent.childrenPIDs; len(got) != 2 || got[0] != 150 || got[1] != 200 {
		t.Fatalf("expected sorted derived child pids, got %#v", got)
	}
}

func TestEnrichProcessInventoryTreeKeepsExistingParentFields(t *testing.T) {
	t.Parallel()

	parent := &processInventorySnapshot{
		pid:                 100,
		name:                "systemd",
		image:               "/usr/lib/systemd/systemd",
		command:             "/usr/lib/systemd/systemd",
		startTimeUnixMillis: 1000,
	}
	child := &processInventorySnapshot{
		pid:                       200,
		parentPID:                 100,
		parentName:                "init",
		parentImage:               "/sbin/init",
		parentCommand:             "/sbin/init",
		parentStartTimeUnixMillis: 900,
		childrenPIDs:              []int{300},
	}

	enrichProcessInventoryTree([]*processInventorySnapshot{parent, child})

	if child.parentName != "init" || child.parentImage != "/sbin/init" || child.parentCommand != "/sbin/init" {
		t.Fatalf("expected existing parent display fields to be preserved, got name=%q image=%q command=%q", child.parentName, child.parentImage, child.parentCommand)
	}
	if child.parentStartTimeUnixMillis != 900 {
		t.Fatalf("expected existing parent start time to be preserved, got %d", child.parentStartTimeUnixMillis)
	}
	if got := child.childrenPIDs; len(got) != 1 || got[0] != 300 {
		t.Fatalf("expected existing child pids to be preserved, got %#v", got)
	}
}

func TestBuildProcessInventoryEventFromSnapshotIncludesParentProcessDetail(t *testing.T) {
	t.Parallel()

	event := buildProcessInventoryEventFromSnapshot(time.Unix(10, 0).UTC(), &processInventorySnapshot{
		pid:                       200,
		parentPID:                 100,
		name:                      "bash",
		image:                     "/usr/bin/bash",
		command:                   "/usr/bin/bash",
		parentName:                "sshd",
		parentImage:               "/usr/sbin/sshd",
		parentCommand:             "/usr/sbin/sshd -D",
		bootID:                    "boot-1",
		startTimeUnixMillis:       2000,
		parentStartTimeUnixMillis: 1000,
	})

	parent, ok := event.Data["parent_process"].(map[string]any)
	if !ok {
		t.Fatalf("expected parent_process detail in event data, got %#v", event.Data)
	}
	if parent["pid"] != 100 || parent["name"] != "sshd" || parent["image"] != "/usr/sbin/sshd" || parent["command"] != "/usr/sbin/sshd -D" || parent["start_time_unix_ms"] != int64(1000) {
		t.Fatalf("unexpected parent_process detail: %#v", parent)
	}
	if event.Process == nil {
		t.Fatal("expected process payload")
	}
	if event.Process.ParentName != "sshd" || event.Process.ParentImage != "/usr/sbin/sshd" || event.Process.ParentCommand != "/usr/sbin/sshd -D" {
		t.Fatalf(
			"expected first-class parent process fields on event payload, got name=%q image=%q command=%q",
			event.Process.ParentName,
			event.Process.ParentImage,
			event.Process.ParentCommand,
		)
	}
}

func TestBuildNetworkInventoryEventSkipsConnectionsWithoutProcessIdentity(t *testing.T) {
	t.Parallel()

	_, ok := buildNetworkInventoryEvent(
		context.Background(),
		time.Unix(10, 0).UTC(),
		"boot-1",
		gopsnet.ConnectionStat{
			Pid:  0,
			Type: 1,
			Laddr: gopsnet.Addr{
				IP:   "10.0.0.5",
				Port: 42000,
			},
			Raddr: gopsnet.Addr{
				IP:   "1.1.1.1",
				Port: 443,
			},
		},
		make(map[int32]*processInventorySnapshot),
	)
	if ok {
		t.Fatal("expected network inventory event without process identity to be skipped")
	}
}
