package log

import (
	rotatelogs "github.com/lestrrat/go-file-rotatelogs"
	"github.com/pkg/errors"
	"io"
	"os"
	"time"
)

func InitFileRotateLogger(baseLogPath string, logSaveDay int, logRotateHour int) error {
	writer, err := rotatelogs.New(
		baseLogPath+".%Y-%m-%d_%H_%M",
		rotatelogs.WithLinkName(baseLogPath),                                // 生成软链，指向最新日志文件
		rotatelogs.WithMaxAge(time.Duration(logSaveDay)*24*time.Hour),       // 文件最大保存时间
		rotatelogs.WithRotationTime(time.Duration(logRotateHour)*time.Hour), // 日志切割时间间隔
	)
	if err != nil {
		return errors.Errorf("init file rotate logger failed: %s", err)
	}

	w := io.MultiWriter(writer, os.Stdout)
	SetOutput(w)
	return nil
}
