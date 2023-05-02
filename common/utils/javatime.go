package utils

import (
	"time"
	"yaklang/common/utils/jodatime"
)

func JavaTimeFormatter(t time.Time, formatter string) string {
	return jodatime.Format(formatter, t)
}
