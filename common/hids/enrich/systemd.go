//go:build hids && linux

package enrich

import "github.com/coreos/go-systemd/v22/journal"

type SystemdContext struct {
	JournalAvailable bool
}

func DetectSystemdContext() SystemdContext {
	return SystemdContext{
		JournalAvailable: journal.Enabled(),
	}
}
