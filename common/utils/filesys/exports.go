package filesys

var Exports = map[string]any{
	"Recursive": Recursive,

	"onReady":    withYaklangOnStart,
	"onStat":     withYaklangStat,
	"onFileStat": withYaklangFileStat,
	"onDirStat":  withYaklangDirStat,
	"dir":        WithDir,
}
