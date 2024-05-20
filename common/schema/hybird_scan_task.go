package schema

import "github.com/jinzhu/gorm"

type HybridScanTask struct {
	gorm.Model

	TaskId string `gorm:"unique_index"`
	// executing
	// paused
	// done
	Status              string
	Reason              string // user cancel / finished / recover failed so on
	SurvivalTaskIndexes string // 暂停的时候正在执行的任务

	// struct{ https bool; request bytes }[]
	Targets string
	// string[]
	Plugins         string
	TotalTargets    int64
	TotalPlugins    int64
	TotalTasks      int64
	FinishedTasks   int64
	FinishedTargets int64

	ScanConfig []byte

	HybridScanTaskSource string
}
