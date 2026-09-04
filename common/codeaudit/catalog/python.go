package catalog

// PythonFrameworkCatalog lists detectable Python web frameworks.
// ArchTool/ConfigTool point at the language-generic AI tools.
var PythonFrameworkCatalog = []FrameworkSignal{
	{
		Name:        "django",
		Display:     "Django",
		FileMarkers: []string{"manage.py", "wsgi.py", "asgi.py"},
		ContentMarkers: []string{
			"DJANGO_SETTINGS_MODULE",
			"from django",
			"django.core",
		},
		StrongContentMarkers: []string{
			"django.contrib.admin",
			"INSTALLED_APPS",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "flask",
		Display: "Flask",
		ContentMarkers: []string{
			"from flask import",
			"Flask(__name__)",
		},
		StrongContentMarkers: []string{
			"@app.route",
			"@blueprint.route",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "fastapi",
		Display: "FastAPI",
		ContentMarkers: []string{
			"from fastapi import",
			"FastAPI(",
		},
		StrongContentMarkers: []string{
			"@app.get(",
			"@app.post(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "tornado",
		Display: "Tornado",
		ContentMarkers: []string{
			"tornado.web",
			"tornado.ioloop",
		},
		StrongContentMarkers: []string{
			"RequestHandler",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
	{
		Name:    "sqlalchemy",
		Display: "SQLAlchemy",
		ContentMarkers: []string{
			"sqlalchemy",
			"create_engine(",
		},
		StrongContentMarkers: []string{
			"sessionmaker(",
		},
		ArchTool:   "framework_arch_info",
		ConfigTool: "framework_config_audit",
	},
}
