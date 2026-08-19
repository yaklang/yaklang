package scannode

import (
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	pluginv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/plugin/v1"
)

func setupPluginGroupsTestDB(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	db, err := consts.CreateProfileDatabase(dir + "/plugin-groups-test.db")
	if err != nil {
		t.Fatalf("create profile database: %v", err)
	}
	consts.BindProfileDatabase(db, dir+"/plugin-groups-test.db")
	t.Cleanup(func() {
		consts.BindProfileDatabase(nil, "")
	})

	scripts := []*schema.YakScript{
		{ScriptName: "weblogic-rce", Type: "port-scan"},
		{ScriptName: "weblogic-weak-pass", Type: "nuclei"},
		{ScriptName: "weblogic-mitm-only", Type: "mitm"},
		{ScriptName: "weaver-oa-rce", Type: "port-scan"},
		{ScriptName: "wordpress-sqli", Type: "nuclei"},
		{ScriptName: "not-grouped", Type: "port-scan"},
	}
	for _, script := range scripts {
		if err := db.Create(script).Error; err != nil {
			t.Fatalf("create yak script %s: %v", script.ScriptName, err)
		}
	}

	groups := []*schema.PluginGroup{
		{YakScriptName: "weblogic-rce", Group: "Weblogic"},
		{YakScriptName: "weblogic-weak-pass", Group: "Weblogic"},
		{YakScriptName: "weblogic-mitm-only", Group: "Weblogic"},
		{YakScriptName: "weaver-oa-rce", Group: "泛微OA"},
		{YakScriptName: "wordpress-sqli", Group: "Wordpress"},
		{YakScriptName: "weaver-oa-rce", Group: "mcp-group-1783762872931342355"},
		{YakScriptName: "weblogic-rce", Group: "BuiltInGroup", IsPocBuiltIn: true},
		{YakScriptName: "weblogic-rce", Group: "TemporaryGroup", TemporaryId: "page-1"},
	}
	for _, group := range groups {
		group.Hash = group.CalcHash()
		if err := db.Create(group).Error; err != nil {
			t.Fatalf("create plugin group %s/%s: %v", group.Group, group.YakScriptName, err)
		}
	}
}

func TestQueryLocalPluginGroups(t *testing.T) {
	setupPluginGroupsTestDB(t)

	result, err := queryLocalPluginGroups()
	if err != nil {
		t.Fatalf("queryLocalPluginGroups: %v", err)
	}

	groupTotals := make(map[string]*pluginv1.PluginGroupInfo)
	for _, group := range result.GetGroups() {
		groupTotals[group.GetGroup()] = group
	}

	if got := groupTotals["Weblogic"]; got == nil {
		t.Fatalf("Weblogic group missing, got groups: %v", groupTotals)
	} else {
		if got.GetTotal() != 3 {
			t.Fatalf("Weblogic total = %d, want 3", got.GetTotal())
		}
		if got.GetCompatibleTotal() != 2 {
			t.Fatalf("Weblogic compatible_total = %d, want 2 (mitm excluded)", got.GetCompatibleTotal())
		}
	}

	if got := groupTotals["泛微OA"]; got == nil {
		t.Fatalf("泛微OA group missing")
	} else if got.GetCompatibleTotal() != 1 || got.GetTotal() != 1 {
		t.Fatalf("泛微OA totals = %d/%d, want 1/1", got.GetCompatibleTotal(), got.GetTotal())
	}

	for _, excluded := range []string{"mcp-group-1783762872931342355", "BuiltInGroup", "TemporaryGroup"} {
		if _, ok := groupTotals[excluded]; ok {
			t.Fatalf("group %s should be excluded", excluded)
		}
	}

	scriptsByGroup := make(map[string][]string)
	for _, script := range result.GetScripts() {
		scriptsByGroup[script.GetGroup()] = append(scriptsByGroup[script.GetGroup()], script.GetScriptName())
	}
	if got := scriptsByGroup["Weblogic"]; len(got) != 3 {
		t.Fatalf("Weblogic scripts = %v, want 3 entries", got)
	}
	if _, ok := scriptsByGroup["not-grouped"]; ok {
		t.Fatalf("ungrouped script leaked into result")
	}
}

func TestValidatePluginGroupsListCommand(t *testing.T) {
	newCommand := func() *pluginv1.ListPluginGroupsCommand {
		return &pluginv1.ListPluginGroupsCommand{
			Metadata:     &nodev1.CommandMetadata{CommandId: "cmd-groups-1"},
			TargetNodeId: "node-a",
		}
	}

	if err := validatePluginGroupsListCommand("node-a", newCommand()); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	command := newCommand()
	command.Metadata = nil
	if err := validatePluginGroupsListCommand("node-a", command); err == nil {
		t.Fatal("missing metadata should fail")
	}

	command = newCommand()
	command.TargetNodeId = "node-b"
	if err := validatePluginGroupsListCommand("node-a", command); err == nil {
		t.Fatal("target node mismatch should fail")
	}
}

func TestIsPortScanCompatiblePluginType(t *testing.T) {
	for _, compatible := range []string{"port-scan", "nuclei", "NASL", " nuclei "} {
		if !isPortScanCompatiblePluginType(compatible) {
			t.Fatalf("type %q should be compatible", compatible)
		}
	}
	for _, incompatible := range []string{"mitm", "yak", "codec", "", "portscan"} {
		if isPortScanCompatiblePluginType(incompatible) {
			t.Fatalf("type %q should be incompatible", incompatible)
		}
	}
}
