//go:build irify_exclude

package yakurl

// createIrifyAction 在 irify_exclude 模式下返回 nil
// 这个函数在 irify_exclude 模式下可用，但不会创建任何 action
func createIrifyAction(schema string) Action {
	return nil
}

