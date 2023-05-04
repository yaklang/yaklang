package permutil

import (
	"os"
)

func IsFileUnreadAndUnWritable(f string) bool {
	fp, err := os.OpenFile(f, os.O_RDWR, 0666)
	if err != nil {
		return os.IsPermission(err)
	}
	fp.Close()
	return false
}
