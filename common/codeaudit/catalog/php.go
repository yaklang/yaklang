package catalog

// PHPFrameworkCatalog lists detectable PHP frameworks.
var PHPFrameworkCatalog = []FrameworkSignal{
	{
		Name:        "laravel",
		Display:     "Laravel",
		FileMarkers: []string{"artisan"},
		ContentMarkers: []string{
			"Illuminate\\",
			"laravel/framework",
		},
		StrongContentMarkers: []string{
			"Illuminate\\Foundation",
			"App\\Providers",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "thinkphp",
		Display: "ThinkPHP",
		ContentMarkers: []string{
			"think\\",
			"ThinkPHP",
		},
		StrongContentMarkers: []string{
			"think\\Controller",
			"think\\Facade",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "wordpress",
		Display: "WordPress",
		FileMarkers: []string{
			"wp-config",
			"wp-settings.php",
		},
		ContentMarkers: []string{
			"wp-content",
			"ABSPATH",
		},
		StrongContentMarkers: []string{
			"DB_PASSWORD",
			"WPLANG",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
}

// PHPCmsCatalog holds PHP product fingerprints. WordPress doubles as a
// framework signal above; the fingerprint drives the CMS audit flow.
var PHPCmsCatalog = []CmsFingerprint{
	{
		ID:      "wordpress",
		Display: "WordPress",
		Family:  "wordpress",
		FileMarkers: []string{
			"wp-config.php",
			"wp-settings.php",
			"wp-content",
		},
		ContentMarkers: []string{
			`DB_PASSWORD`,
			`ABSPATH`,
			`wp_version`,
		},
	},
}
