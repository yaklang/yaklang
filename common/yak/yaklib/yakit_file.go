package yaklib

import (
	"encoding/json"
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/filemode"
)

type YakitFileAction struct {
	Action  string
	Message map[string]any
}

var (
	Read_Action   = "READ"
	Write_Action  = "WRITE"
	Create_Action = "CREATE"
	Delete_Action = "DELETE"
	Status_Action = "STATUS"
	Chmod_Action  = "CHMOD"
	Find_Action   = "FIND"
)

func FileReadAction(offset int, length int, unit string, content []byte) *YakitFileAction {
	return &YakitFileAction{
		Action: Read_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("read file with offset %d, length %d, unit %s", offset, length, unit),
			"offset":  offset,
			"length":  length,
			"unit":    unit,
			"content": content,
		},
	}
}

func FileWriteAction(length int, mode string, content []byte) *YakitFileAction {
	return &YakitFileAction{
		Action: Write_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("write file with length %d, mode %s", length, mode),
			"length":  length,
			"mode":    mode, // append | cover
			"content": content,
		},
	}
}

func FileCreateAction(isDir bool, chmodMode uint32) *YakitFileAction {
	return &YakitFileAction{
		Action: Create_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("create file, isDir: %t, chmodMode: %s", isDir, filemode.FileMode(chmodMode).String()),
			"isDir":     isDir,
			"chmodMode": filemode.FileMode(chmodMode).String(),
		},
	}
}

func FileDeleteAction(isDir bool) *YakitFileAction {
	return &YakitFileAction{
		Action: Delete_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("delete file, isDir: %t", isDir),
			"isDir":   isDir,
		},
	}
}

func FileStatusAction(status any) *YakitFileAction {
	return &YakitFileAction{
		Action: Status_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("get file status: %v", status),
			"status":  status,
		},
	}
}

func FileChmodAction(chmodMode uint32) *YakitFileAction {
	return &YakitFileAction{
		Action: Chmod_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("change file mode to %s", filemode.FileMode(chmodMode).String()),
			"chmodMode": filemode.FileMode(chmodMode).String(),
		},
	}
}

func FileFindAction(findMode string, findCondition string, findContent ...string) *YakitFileAction {
	return &YakitFileAction{
		Action: Find_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("find file [mode:%s] [condition:%s]", findMode, findCondition),
			"mode":      findMode, // name | content | all
			"content":   findContent,
			"condition": findCondition,
		},
	}
}

func (a *YakitFileAction) String() string {
	actionString, err := json.Marshal(a)
	if err != nil {
		return ""
	}
	return string(actionString)
}

func init() {
	YakitExports["fileReadAction"] = FileReadAction
	YakitExports["fileWriteAction"] = FileWriteAction
	YakitExports["fileCreateAction"] = FileCreateAction
	YakitExports["fileDeleteAction"] = FileDeleteAction
	YakitExports["fileStatusAction"] = FileStatusAction
	YakitExports["fileChmodAction"] = FileChmodAction
	YakitExports["fileFindAction"] = FileFindAction
}
