package catalog

// NodeFrameworkCatalog lists detectable Node.js server frameworks.
var NodeFrameworkCatalog = []FrameworkSignal{
	{
		Name:    "express",
		Display: "Express",
		ContentMarkers: []string{
			"require('express')",
			"require(\"express\")",
			"from 'express'",
			"from \"express\"",
		},
		StrongContentMarkers: []string{
			"express()",
			"app.listen(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "koa",
		Display: "Koa",
		ContentMarkers: []string{
			"require('koa')",
			"require(\"koa\")",
			"from 'koa'",
			"from \"koa\"",
		},
		StrongContentMarkers: []string{
			"new Koa(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "nestjs",
		Display: "NestJS",
		ContentMarkers: []string{
			"@nestjs/core",
			"@nestjs/common",
		},
		StrongContentMarkers: []string{
			"@Controller(",
			"@Module(",
			"NestFactory.create",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
}
