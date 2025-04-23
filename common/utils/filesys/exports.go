package filesys

var Exports = map[string]any{
	"CopyToTemporary": CopyToTemporary,
	"CopyToRefLocal":  CopyToRefLocal,
	"Recursive":       Recursive,

	"onFS":         withYaklangFileSystem,
	"onReady":      withYaklangOnStart,
	"onStat":       withYaklangStat,
	"onStatEx":     withYaklangStatEx,
	"onFileStat":   withYaklangFileStat,
	"onFileStatEx": withYaklangFileStatEx,
	"onDirStat":    withYaklangDirStat,
	"dir":          WithDir,
}
