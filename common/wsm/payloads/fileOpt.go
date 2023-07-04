package payloads

type FileOpt string

var (
	List            FileOpt = "list"
	Show            FileOpt = "show"
	Delete          FileOpt = "delete"
	Create          FileOpt = "create"
	Append          FileOpt = "append"
	Download        FileOpt = "download"
	Rename          FileOpt = "rename"
	CreateFile      FileOpt = "createFile"
	CreateDirectory FileOpt = "createDirectory"
	GetTimeStamp    FileOpt = "getTimeStamp"
	UpdateTimeStamp FileOpt = "updateTimeStamp"
)

type FileInfo struct {
	Path string `json:"path"`

	NewPath string `json:"newPath"`

	Charset string `json:"charset"`

	CreateTimeStamp string `json:"createTimeStamp"`
	AccessTimeStamp string `json:"accessTimeStamp"`
	ModifyTimeStamp string `json:"modifyTimeStamp"`
}
