package java_decompiler

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakurl/base"
)

// Action represents the Java decompiler action
type Action struct {
	base.BaseAction

	// jarFS maps JAR paths to their filesystem handlers
	jarFS *utils.SafeMap[*javaclassparser.JarFS]
}

// NewJavaDecompilerAction creates and initializes a new Java decompiler action
func NewJavaDecompilerAction() *Action {
	action := &Action{
		jarFS: utils.NewSafeMap[*javaclassparser.JarFS](),
	}

	// Register route handlers
	action.registerJarRoutes()
	action.registerClassRoutes()

	return action
}

// ClearCache clears all cached JarFS instances
// This is useful for testing to release file handles on Windows
func (a *Action) ClearCache() {
	a.jarFS.Clear()
}
