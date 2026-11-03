package ssautils

import (
	"github.com/yaklang/javajive/classparser"
)

// OpenJarFS opens a local jar/war as a decompiling filesystem.
//
// Retaining unproved sources is the frontend contract: when javac cannot
// recompile a decompiler spelling, the original stays visible instead of being
// replaced by a failure comment. Keeping both calls here means ssaapi never
// touches the decompiler API — archive handling belongs to source preparation.
func OpenJarFS(local string, recursive bool) (*javaclassparser.JarFS, error) {
	jarFS, err := javaclassparser.NewJarFSFromLocalWithOptions(local, recursive)
	if err != nil {
		return nil, err
	}
	jarFS.SetRetainUnprovedSource(true)
	return jarFS, nil
}
