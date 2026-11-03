package aiskillloader

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func TestDefaultSkillInventoryShowsOnlyConfiguredNames(t *testing.T) {
	loader := &metadataOnlySkillLoader{metas: []*SkillMeta{
		{Name: "coding-workflow", Description: "coding"},
		{Name: "security-skill", Description: "security"},
		{Name: "yaklang-system", Description: "system"},
	}}
	manager := NewSkillsContextManager(
		loader,
		WithDefaultSkillInventory("coding-workflow", "yaklang-system"),
	)

	inventory := manager.SkillInventory()
	require.Contains(t, inventory, "coding-workflow")
	require.Contains(t, inventory, "yaklang-system")
	require.NotContains(t, inventory, "security-skill")
	require.Less(t, strings.Index(inventory, "coding-workflow"), strings.Index(inventory, "yaklang-system"))
}

type metadataOnlySkillLoader struct {
	metas []*SkillMeta
}

func (l *metadataOnlySkillLoader) LoadSkill(name string) (*LoadedSkill, error) {
	return nil, nil
}

func (l *metadataOnlySkillLoader) GetFileSystem(string) (fi.FileSystem, error) {
	return nil, nil
}

func (l *metadataOnlySkillLoader) HasSkills() bool { return len(l.metas) > 0 }

func (l *metadataOnlySkillLoader) AllSkillMetas() []*SkillMeta { return l.metas }
