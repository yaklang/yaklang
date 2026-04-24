package mcp

import "testing"

func TestProjectDatabaseToolSetRegistered(t *testing.T) {
	set, ok := globalToolSets["project_database"]
	if !ok {
		t.Fatalf("project_database tool set not registered")
	}
	for _, name := range []string{
		"get_current_database_context",
		"list_project_databases",
		"switch_current_project_database",
		"create_project_database",
	} {
		if _, exists := set.Tools[name]; !exists {
			t.Fatalf("tool not registered: %s", name)
		}
	}
}
