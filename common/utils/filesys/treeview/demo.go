package treeview

import (
	"github.com/yaklang/yaklang/common/log"
)

// DemoTreeViewLimits 演示 TreeView 的深度和行数限制功能
func DemoTreeViewLimits() {
	log.Info("Demonstrating TreeView depth and line limits functionality")

	// 创建一个深层嵌套的目录结构用于演示
	paths := []string{
		"project/src/main/java/com/example/App.java",
		"project/src/main/java/com/example/service/UserService.java",
		"project/src/main/java/com/example/service/OrderService.java",
		"project/src/main/java/com/example/controller/UserController.java",
		"project/src/main/java/com/example/controller/OrderController.java",
		"project/src/main/resources/application.yml",
		"project/src/main/resources/db/migration/V1__init.sql",
		"project/src/test/java/com/example/AppTest.java",
		"project/src/test/java/com/example/service/UserServiceTest.java",
		"project/pom.xml",
		"project/README.md",
		"project/docs/api.md",
		"project/docs/setup.md",
		"project/docs/architecture.md",
		"project/scripts/build.sh",
		"project/scripts/deploy.sh",
		"project/scripts/test.sh",
		"project/config/dev.yml",
		"project/config/prod.yml",
		"project/config/test.yml",
	}

	log.Info("=== Original TreeView (no limits) ===")
	tv1 := NewTreeView(paths)
	log.Infof("Tree structure:\n%s", tv1.Print())

	log.Info("=== TreeView with depth limit 3 ===")
	tv2 := NewTreeViewWithLimits(paths, 3, 0)
	log.Infof("Depth limited tree:\n%s", tv2.Print())

	log.Info("=== TreeView with line limit 10 ===")
	tv3 := NewTreeViewWithLimits(paths, 0, 10)
	log.Infof("Line limited tree:\n%s", tv3.Print())

	log.Info("=== TreeView with both depth=2 and line=8 limits ===")
	tv4 := NewTreeViewWithLimits(paths, 2, 8)
	log.Infof("Both limited tree:\n%s", tv4.Print())

	log.Info("=== TreeView with single folder collapse ===")
	tv5 := NewTreeViewWithOptions(paths, 0, 0, true)
	log.Infof("Collapsed tree:\n%s", tv5.Print())

	// 统计信息
	files, dirs := tv1.Count()
	log.Infof("Total files: %d, directories: %d", files, dirs)
}
