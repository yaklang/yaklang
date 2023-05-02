package utils

import (
	"time"
	"github.com/yaklang/yaklang/common/utils/jodatime"
)

func JavaTimeFormatter(t time.Time, formatter string) string {
	return jodatime.Format(formatter, t)
}
