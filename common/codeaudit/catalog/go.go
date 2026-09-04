package catalog

// GoFrameworkCatalog lists detectable Go web frameworks and ORMs.
var GoFrameworkCatalog = []FrameworkSignal{
	{
		Name:    "gin",
		Display: "Gin",
		ContentMarkers: []string{
			"github.com/gin-gonic/gin",
			"gin.Default()",
			"gin.New()",
		},
		StrongContentMarkers: []string{
			"gin.Engine",
			".GET(\"/",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "echo",
		Display: "Echo",
		ContentMarkers: []string{
			"github.com/labstack/echo",
			"echo.Start(",
		},
		StrongContentMarkers: []string{
			"echo.New()",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "beego",
		Display: "Beego",
		ContentMarkers: []string{
			"github.com/beego/beego",
			"beego.Controller",
		},
		StrongContentMarkers: []string{
			"beego.Run(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "fiber",
		Display: "Fiber",
		ContentMarkers: []string{
			"github.com/gofiber/fiber",
			"fiber.New(",
		},
		StrongContentMarkers: []string{
			"app.Listen(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "gorm",
		Display: "GORM",
		ContentMarkers: []string{
			"gorm.io/gorm",
			"gorm.Open(",
		},
		StrongContentMarkers: []string{
			"gorm.Model",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
}
