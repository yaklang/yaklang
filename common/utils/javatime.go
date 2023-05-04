package utils

import (
	"github.com/yaklang/yaklang/common/utils/jodatime"
	"time"
)

func JavaTimeFormatter(t time.Time, formatter string) string {
	return jodatime.Format(formatter, t)
}
