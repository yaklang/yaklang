package sfdb

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestCreateGroupConcurrentSameNameIsIdempotent(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// Match production SQLite's single-writer pool while keeping callers concurrent.
	db.DB().SetMaxOpenConns(1)
	db.DB().SetMaxIdleConns(1)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error)

	name := "concurrent-" + uuid.NewString()
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := CreateGroup(db, name, true)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, db.Model(&schema.SyntaxFlowGroup{}).Where("group_name = ?", name).Count(&count).Error)
	require.Equal(t, int64(1), count)

	groups := GetOrCreateGroups(db, []string{name, name})
	require.Len(t, groups, 2)
	require.Equal(t, name, groups[0].GroupName)
}

func TestMigrateSyntaxFlowDoesNotCascadeCreateGroups(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.SyntaxFlowGroup{}, &schema.SyntaxFlowRule{}).Error)

	groupName := "java-" + uuid.NewString()
	_, err = CreateGroup(db, groupName, true)
	require.NoError(t, err)

	rule := &schema.SyntaxFlowRule{
		RuleName: "rule-" + uuid.NewString(),
		Content:  `desc(title: "t") alert $a`,
		Groups:   []*schema.SyntaxFlowGroup{{GroupName: groupName}},
	}
	require.NoError(t, MigrateSyntaxFlowWithDB(db, "", rule))
	require.Len(t, rule.Groups, 1, "caller Groups slice must be restored")

	var groupCount int64
	require.NoError(t, db.Model(&schema.SyntaxFlowGroup{}).Where("group_name = ?", groupName).Count(&groupCount).Error)
	require.Equal(t, int64(1), groupCount)

	var ruleCount int64
	require.NoError(t, db.Model(&schema.SyntaxFlowRule{}).Where("rule_name = ?", rule.RuleName).Count(&ruleCount).Error)
	require.Equal(t, int64(1), ruleCount)
}
