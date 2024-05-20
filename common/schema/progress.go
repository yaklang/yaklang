package schema

import "github.com/jinzhu/gorm"

type Progress struct {
	gorm.Model
	RuntimeId            string
	CurrentProgress      float64
	YakScriptOnlineGroup string
	// 记录指针
	LastRecordPtr int64
	TaskName      string
	// 额外信息
	ExtraInfo string

	ProgressSource string

	// 任务记录的参数
	ProgressTaskParam []byte

	// 目标 大部分的progress都应该有制定目标，所以尝试提取出来作为单独的数据使用
	Target string
}
