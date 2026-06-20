package yaklib

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/yaklang/yaklang/common/utils"
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

// fileReadAction 构造一个「读文件」操作记录（导出名为 yakit.fileReadAction）
// 用于在 Agent/插件执行文件读操作时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - offset: 读取的起始偏移
//   - length: 读取长度
//   - unit: 长度单位（如 "byte"、"line"）
//   - content: 读到的内容（过长会被截断展示）
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileReadAction(0, 5, "byte", []byte("hello"))
// println(action.Action)   // OUT: READ
// assert action.Action == "READ", "fileReadAction should build a READ action"
// ```
func FileReadAction(offset int, length int, unit string, content []byte) *YakitFileAction {
	return &YakitFileAction{
		Action: Read_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("read file with offset %d, length %d, unit %s", offset, length, unit),
			"offset":  offset,
			"length":  length,
			"unit":    unit,
			"content": utils.ShrinkTextBlock(utils.InterfaceToString(content), 4096),
		},
	}
}

// fileWriteAction 构造一个「写文件」操作记录（导出名为 yakit.fileWriteAction）
// 用于在 Agent/插件执行文件写操作时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - length: 写入长度
//   - mode: 写入模式（append 追加 | cover 覆盖）
//   - content: 写入的内容
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileWriteAction(5, "cover", []byte("hello"))
// println(action.Action)   // OUT: WRITE
// assert action.Action == "WRITE", "fileWriteAction should build a WRITE action"
// ```
func FileWriteAction(length int, mode string, content []byte) *YakitFileAction {
	return &YakitFileAction{
		Action: Write_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("write file with length %d, mode %s", length, mode),
			"length":  length,
			"mode":    mode, // append | cover
			"content": utils.InterfaceToString(content),
		},
	}
}

// fileCreateAction 构造一个「创建文件/目录」操作记录（导出名为 yakit.fileCreateAction）
// 用于在 Agent/插件创建文件或目录时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - isDir: 是否为目录
//   - chmodMode: 权限位（如 0o644）
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileCreateAction(false, 0o644)
// println(action.Action)   // OUT: CREATE
// assert action.Action == "CREATE", "fileCreateAction should build a CREATE action"
// ```
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

// fileDeleteAction 构造一个「删除文件/目录」操作记录（导出名为 yakit.fileDeleteAction）
// 用于在 Agent/插件删除文件或目录时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - isDir: 是否为目录
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileDeleteAction(false)
// println(action.Action)   // OUT: DELETE
// assert action.Action == "DELETE", "fileDeleteAction should build a DELETE action"
// ```
func FileDeleteAction(isDir bool) *YakitFileAction {
	return &YakitFileAction{
		Action: Delete_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("delete file, isDir: %t", isDir),
			"isDir":   isDir,
		},
	}
}

// fileStatusAction 构造一个「文件状态」操作记录（导出名为 yakit.fileStatusAction）
// 用于上报文件的元信息；若传入 os.FileInfo 会自动提取 name/size/mode/modTime/isDir
//
// 参数:
//   - status: 文件状态，支持 os.FileInfo 或可转为 map 的对象
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileStatusAction({"name": "a.txt", "size": 10})
// println(action.Action)   // OUT: STATUS
// assert action.Action == "STATUS", "fileStatusAction should build a STATUS action"
// ```
func FileStatusAction(status any) *YakitFileAction {
	statusMap := make(map[string]any)
	fileInfo, ok := status.(os.FileInfo)
	if ok {
		statusMap["name"] = fileInfo.Name()
		statusMap["size"] = fileInfo.Size()
		statusMap["mode"] = fileInfo.Mode().String()
		statusMap["modTime"] = fileInfo.ModTime().String()
		statusMap["isDir"] = fileInfo.IsDir()
	} else {
		statusMap = utils.InterfaceToGeneralMap(status)
	}
	return &YakitFileAction{
		Action: Status_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("get file status: %+v", statusMap),
			"status":  statusMap,
		},
	}
}

// fileChmodAction 构造一个「修改文件权限」操作记录（导出名为 yakit.fileChmodAction）
// 用于在 Agent/插件修改文件权限时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - chmodMode: 新的权限位（如 0o755）
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileChmodAction(0o755)
// println(action.Action)   // OUT: CHMOD
// assert action.Action == "CHMOD", "fileChmodAction should build a CHMOD action"
// ```
func FileChmodAction(chmodMode uint32) *YakitFileAction {
	return &YakitFileAction{
		Action: Chmod_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("change file mode to %s", filemode.FileMode(chmodMode).String()),
			"chmodMode": filemode.FileMode(chmodMode).String(),
		},
	}
}

// fileFindAction 构造一个「查找文件」操作记录（导出名为 yakit.fileFindAction）
// 用于在 Agent/插件执行文件查找时，向 Yakit 上报结构化的操作信息
//
// 参数:
//   - findMode: 查找模式（name 按名 | content 按内容 | all 全部）
//   - findCondition: 查找条件
//   - findContent: 可选的查找内容
//
// 返回值:
//   - 文件操作记录对象
//
// Example:
// ```
// action = yakit.fileFindAction("name", "*.go")
// println(action.Action)   // OUT: FIND
// assert action.Action == "FIND", "fileFindAction should build a FIND action"
// ```
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
