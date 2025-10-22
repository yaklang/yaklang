package schema

var (
	actionI18n = map[string]*I18n{
		"continue-current-task": {
			Zh: "继续当前任务",
			En: "Continue current task",
		},
		"proceed-next-task": {
			Zh: "继续下一个任务",
			En: "Proceed to next task",
		},
		"task-failed": {
			Zh: "任务失败",
			En: "Task failed",
		},
		"task-skipped": {
			Zh: "任务跳过",
			En: "Task skipped",
		},
	}
)

func GetActionI18n(action string) *I18n {
	return actionI18n[action]
}
