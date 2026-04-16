//go:build hids && linux

package runtime

import "time"

type health struct {
	status    string
	message   string
	updatedAt time.Time
}

func newHealth(status string, message string) health {
	return health{
		status:    status,
		message:   message,
		updatedAt: time.Now().UTC(),
	}
}
