package yakit

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
)

// Create test custom code signing data
func createTestSnippets() *schema.Snippets {
	return &schema.Snippets{
		CustomCodeName:  uuid.NewString(),
		CustomCodeData:  uuid.NewString(),
		CustomCodeDesc:  uuid.NewString(),
		CustomCodeState: "none",
		CustomCodeLevel: "none",
	}
}

func TestCreateSnippets(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	t.Run("Successfully create custom code signing", func(t *testing.T) {
		customCode := createTestSnippets()
		defer DeleteSnippetsByName(db, customCode.CustomCodeName)

		err := CreateSnippet(db, customCode)
		require.NoError(t, err)
		assert.NotZero(t, customCode.ID)
		assert.NotZero(t, customCode.CreatedAt)
		assert.NotZero(t, customCode.UpdatedAt)
	})

	t.Run("Creating custom code signing with duplicate name should fail", func(t *testing.T) {
		customCode1 := createTestSnippets()
		customCode2 := &schema.Snippets{
			CustomCodeName:  customCode1.CustomCodeName,
			CustomCodeData:  uuid.NewString(),
			CustomCodeDesc:  "",
			CustomCodeState: "none",
		}
		defer DeleteSnippetsByName(db, customCode1.CustomCodeName)
		defer DeleteSnippetsByName(db, customCode2.CustomCodeName)

		// Create the first one first
		err := CreateSnippet(db, customCode1)
		require.NoError(t, err)

		// Try to create the second one with the same name
		err = CreateSnippet(db, customCode2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("Creating custom code signing with empty name should fail", func(t *testing.T) {
		customCode := &schema.Snippets{
			CustomCodeName:  "",
			CustomCodeData:  uuid.NewString(),
			CustomCodeDesc:  "",
			CustomCodeState: "none",
		}
		defer DeleteSnippetsByName(db, customCode.CustomCodeName)

		err := CreateSnippet(db, customCode)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})
}

func TestGetSnippetsByName(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	t.Run("Successfully get custom code signing by name", func(t *testing.T) {
		// Create one first
		customCode := createTestSnippets()
		defer DeleteSnippetsByName(db, customCode.CustomCodeName)

		err := CreateSnippet(db, customCode)
		require.NoError(t, err)

		// Get by name
		retrieved, err := GetSnippetsByName(db, customCode.CustomCodeName)
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, customCode.CustomCodeName, retrieved.CustomCodeName)
		assert.Equal(t, customCode.CustomCodeData, retrieved.CustomCodeData)
	})

	t.Run("Getting non-existent name should fail", func(t *testing.T) {
		retrieved, err := GetSnippetsByName(db, uuid.NewString())
		require.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Passing empty name should fail", func(t *testing.T) {
		retrieved, err := GetSnippetsByName(db, "")
		require.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "cannot be empty")
	})
}

func TestUpdateSnippets(t *testing.T) {
	db := consts.GetGormProjectDatabase()

	t.Run("Successfully update custom code signing", func(t *testing.T) {
		// Create one first
		customCode := createTestSnippets()
		defer DeleteSnippetsByName(db, customCode.CustomCodeName)
		err := CreateSnippet(db, customCode)
		require.NoError(t, err)

		// Update data
		customCode.CustomCodeData = uuid.NewString()
		err = UpdateSnippet(db, customCode.CustomCodeName, customCode)
		require.NoError(t, err)

		// Verify update
		retrieved, err := GetSnippetsByName(db, customCode.CustomCodeName)
		require.NoError(t, err)
		assert.Equal(t, customCode.CustomCodeData, retrieved.CustomCodeData)
	})

	t.Run("Updating non-existent record should fail", func(t *testing.T) {
		customCode := createTestSnippets()
		err := UpdateSnippet(db, customCode.CustomCodeName, customCode)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetAllSnippetss(t *testing.T) {
	t.Skip()
	db := consts.GetGormProjectDatabase()

	t.Run("Get all custom code signings", func(t *testing.T) {
		// Create a few first
		customCode1 := createTestSnippets()
		customCode2 := createTestSnippets()
		defer DeleteSnippetsByName(db, customCode1.CustomCodeName)
		defer DeleteSnippetsByName(db, customCode2.CustomCodeName)

		err := CreateSnippet(db, customCode1)
		require.NoError(t, err)
		err = CreateSnippet(db, customCode2)
		require.NoError(t, err)

		// Get all
		all, err := GetAllSnippetss(db)
		require.NoError(t, err)
		assert.Len(t, all, 2)

		// Check if it contains the data we created
		names := make(map[string]bool)
		for _, code := range all {
			names[code.CustomCodeName] = true
		}
		assert.True(t, names[customCode1.CustomCodeName])
		assert.True(t, names[customCode2.CustomCodeName])
	})

	t.Run("Should return empty slice when database is empty", func(t *testing.T) {
		all, err := GetAllSnippetss(db)
		require.NoError(t, err)
		assert.Len(t, all, 0)
	})
}

func TestGetSnippetssWithPagination(t *testing.T) {
	t.Skip()
	db := consts.GetGormProjectDatabase()

	t.Run("Get custom code signings with pagination", func(t *testing.T) {
		// Create multiple test data
		for i := 1; i <= 25; i++ {
			customCode := &schema.Snippets{
				CustomCodeName:  fmt.Sprintf("test_code_%s", uuid.NewString()),
				CustomCodeData:  fmt.Sprintf("test_data_%s", uuid.NewString()),
				CustomCodeDesc:  "",
				CustomCodeState: "none",
			}
			err := CreateSnippet(db, customCode)
			defer DeleteSnippetsByName(db, customCode.CustomCodeName)
			require.NoError(t, err)
		}

		// Test first page
		page1, total, err := GetSnippetssWithPagination(db, 1, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(25), total)
		assert.Len(t, page1, 10)

		// Test second page
		page2, total, err := GetSnippetssWithPagination(db, 2, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(25), total)
		assert.Len(t, page2, 10)

		// Test last page
		page3, total, err := GetSnippetssWithPagination(db, 3, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(25), total)
		assert.Len(t, page3, 5)
	})

	t.Run("Pagination parameter boundary cases", func(t *testing.T) {
		// Test invalid page number
		_, _, err := GetSnippetssWithPagination(db, 0, 10)
		require.NoError(t, err) // Function will correct to 1 internally

		// Test invalid page size
		_, _, err = GetSnippetssWithPagination(db, 1, 0)
		require.NoError(t, err) // Function will correct to 10 internally
	})

	t.Run("Passing nil database connection should fail", func(t *testing.T) {
		_, _, err := GetSnippetssWithPagination(nil, 1, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database connection is nil")
	})
}
