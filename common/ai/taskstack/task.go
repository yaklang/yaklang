package taskstack

import (
	_ "embed"
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed jsonschema/task.json
var taskJsonSchema string

type Task struct {
	Name     string
	Goal     string
	Subtasks []Task
}

type Runtime struct {
	Freeze bool
	Task   Task
	Stack  *utils.Stack[Task]
}

func ExtractTaskFromRawResponse(rawResponse string) (*Task, error) {
	for _, item := range jsonextractor.ExtractObjectIndexes(rawResponse) {
		start, end := item[0], item[1]
		taskJSON := rawResponse[start:end]

		// 尝试解析为完整的 task 结构
		var taskObj struct {
			Tasks []struct {
				SubtaskName string `json:"subtask_name"`
				SubtaskGoal string `json:"subtask_goal"`
			} `json:"tasks"`
		}

		err := json.Unmarshal([]byte(taskJSON), &taskObj)
		if err == nil && len(taskObj.Tasks) > 0 {
			// 找到了合法的任务结构
			mainTask := &Task{
				Name:     taskObj.Tasks[0].SubtaskName,
				Goal:     taskObj.Tasks[0].SubtaskGoal,
				Subtasks: make([]Task, 0),
			}

			// 如果有多个任务，将后续任务作为子任务
			if len(taskObj.Tasks) > 1 {
				for _, subtask := range taskObj.Tasks[1:] {
					if subtask.SubtaskName != "" {
						mainTask.Subtasks = append(mainTask.Subtasks, Task{
							Name: subtask.SubtaskName,
							Goal: subtask.SubtaskGoal,
						})
					}
				}
			}

			// 检查主任务 Name 是否存在
			if mainTask.Name != "" {
				return mainTask, nil
			}
		}

		// 尝试直接解析为单个 Task 对象
		var simpleTask Task
		err = json.Unmarshal([]byte(taskJSON), &simpleTask)
		if err == nil && simpleTask.Name != "" {
			return &simpleTask, nil
		}

		// 尝试解析为一个简单的 map 并创建 Task
		var taskMap map[string]interface{}
		err = json.Unmarshal([]byte(taskJSON), &taskMap)
		if err == nil {
			if name, ok := taskMap["name"].(string); ok && name != "" {
				task := &Task{
					Name: name,
				}

				if goal, ok := taskMap["goal"].(string); ok {
					task.Goal = goal
				}

				if subtasks, ok := taskMap["subtasks"].([]interface{}); ok {
					for _, st := range subtasks {
						if subtaskMap, ok := st.(map[string]interface{}); ok {
							if stName, ok := subtaskMap["name"].(string); ok && stName != "" {
								subtask := Task{
									Name: stName,
								}

								if stGoal, ok := subtaskMap["goal"].(string); ok {
									subtask.Goal = stGoal
								}

								task.Subtasks = append(task.Subtasks, subtask)
							}
						}
					}
				}

				return task, nil
			}
		}
	}
	return nil, errors.New("no task found")
}
