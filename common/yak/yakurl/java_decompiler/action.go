package java_decompiler

import (
	"sync"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/yakurl/base"
)

// Action represents the Java decompiler action
type Action struct {
	base.BaseAction

	// mutex protects concurrent access to the jarFS map
	mutex sync.Mutex

	// jarFS maps JAR paths to their filesystem handlers
	jarFS map[string]*javaclassparser.JarFS
}

// NewJavaDecompilerAction creates and initializes a new Java decompiler action
func NewJavaDecompilerAction() *Action {
	action := &Action{
		jarFS: make(map[string]*javaclassparser.JarFS),	
	}

	// Register route handlers
	action.registerJarRoutes()
	action.registerClassRoutes()

	return action
}

// ClearCache clears all cached JarFS instances
// This is useful for testing to release file handles on Windows
func (a *Action) ClearCache() {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.jarFS = make(map[string]*javaclassparser.JarFS)
}