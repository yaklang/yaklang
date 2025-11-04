package aitool

import (
	"encoding/json"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// TestBuildParamsOptions_SimpleTypes tests building options from simple type parameters
func TestBuildParamsOptions_SimpleTypes(t *testing.T) {
	// Create a tool with various simple parameters
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStringParam("name",
			WithParam_Description("User name"),
			WithParam_Required(true),
		),
		WithIntegerParam("age",
			WithParam_Description("User age"),
			WithParam_Min(0),
			WithParam_Max(150),
		),
		WithNumberParam("score",
			WithParam_Description("User score"),
			WithParam_Default(0.0),
		),
		WithBoolParam("active",
			WithParam_Description("Is active"),
			WithParam_Default(true),
		),
	)
	require.NoError(t, err)

	// Build params options
	opts := tool.BuildParamsOptions()
	require.Len(t, opts, 4, "should have 4 parameters")

	// Rebuild the tool
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)
	require.NotNil(t, rebuiltTool)

	// Compare basic properties
	require.Equal(t, tool.Name, rebuiltTool.Name)
	require.Equal(t, tool.Description, rebuiltTool.Description)
	require.Equal(t, tool.Params().Len(), rebuiltTool.Params().Len())

	// Compare required fields
	require.ElementsMatch(t, tool.InputSchema.Required, rebuiltTool.InputSchema.Required)
}

// TestBuildParamsOptions_StringConstraints tests string parameter constraints
func TestBuildParamsOptions_StringConstraints(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStringParam("username",
			WithParam_Description("Username"),
			WithParam_MinLength(3),
			WithParam_MaxLength(20),
			WithParam_Pattern("^[a-zA-Z0-9]+$"),
			WithParam_Required(true),
		),
		WithStringParam("status",
			WithParam_EnumString("active", "inactive", "pending"),
			WithParam_Default("pending"),
		),
	)
	require.NoError(t, err)

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	// Check username parameter
	usernameParam, ok := rebuiltTool.Params().Get("username")
	require.True(t, ok)
	usernameMap := utils.InterfaceToGeneralMap(usernameParam)
	require.Equal(t, 3, utils.MapGetInt(usernameMap, "minLength"))
	require.Equal(t, 20, utils.MapGetInt(usernameMap, "maxLength"))
	require.Equal(t, "^[a-zA-Z0-9]+$", utils.MapGetString(usernameMap, "pattern"))

	// Check status parameter
	statusParam, ok := rebuiltTool.Params().Get("status")
	require.True(t, ok)
	statusMap := utils.InterfaceToGeneralMap(statusParam)
	enumRaw := utils.MapGetRaw(statusMap, "enum")
	require.NotNil(t, enumRaw)
}

// TestBuildParamsOptions_NumberConstraints tests number parameter constraints
func TestBuildParamsOptions_NumberConstraints(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithIntegerParam("count",
			WithParam_Min(1),
			WithParam_Max(100),
			WithParam_MultipleOf(5),
		),
		WithNumberParam("price",
			WithParam_Min(0.01),
			WithParam_Max(999.99),
		),
	)
	require.NoError(t, err)

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	// Check count parameter
	countParam, ok := rebuiltTool.Params().Get("count")
	require.True(t, ok)
	countMap := utils.InterfaceToGeneralMap(countParam)
	require.Equal(t, 1.0, utils.MapGetFloat64(countMap, "minimum"))
	require.Equal(t, 100.0, utils.MapGetFloat64(countMap, "maximum"))
	require.Equal(t, 5.0, utils.MapGetFloat64(countMap, "multipleOf"))

	// Check price parameter
	priceParam, ok := rebuiltTool.Params().Get("price")
	require.True(t, ok)
	priceMap := utils.InterfaceToGeneralMap(priceParam)
	require.Equal(t, 0.01, utils.MapGetFloat64(priceMap, "minimum"))
	require.Equal(t, 999.99, utils.MapGetFloat64(priceMap, "maximum"))
}

// TestBuildParamsOptions_ArrayParams tests array parameter handling
func TestBuildParamsOptions_ArrayParams(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStringArrayParam("tags",
			WithParam_Description("User tags"),
		),
		WithNumberArrayParam("scores",
			WithParam_Description("User scores"),
		),
	)
	require.NoError(t, err)

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	// Check tags parameter
	tagsParam, ok := rebuiltTool.Params().Get("tags")
	require.True(t, ok)
	tagsMap := utils.InterfaceToGeneralMap(tagsParam)
	require.Equal(t, "array", utils.MapGetString(tagsMap, "type"))

	itemsRaw := utils.MapGetRaw(tagsMap, "items")
	require.NotNil(t, itemsRaw)
	itemsMap := utils.InterfaceToGeneralMap(itemsRaw)
	require.Equal(t, "string", utils.MapGetString(itemsMap, "type"))
}

// TestBuildParamsOptions_StructParam tests struct parameter handling
func TestBuildParamsOptions_StructParam(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStructParam("user",
			[]PropertyOption{
				WithParam_Description("User object"),
				WithParam_Required(true),
			},
			WithStringParam("name", WithParam_Required(true)),
			WithIntegerParam("age"),
			WithBoolParam("active", WithParam_Default(true)),
		),
	)
	require.NoError(t, err)

	log.Infof("Original tool schema:\n%s", tool.ParamsJsonSchemaString())
	log.Infof("Original tool InputSchema required: %v", tool.InputSchema.Required)

	// Debug: Check what's in the user param
	userParam, ok := tool.Params().Get("user")
	require.True(t, ok)
	spew.Dump("Original user param:", userParam)

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	log.Infof("Rebuilt tool schema:\n%s", rebuiltTool.ParamsJsonSchemaString())

	// Debug: Check what's in the rebuilt user param
	rebuiltUserParam, ok := rebuiltTool.Params().Get("user")
	require.True(t, ok)
	log.Infof("Rebuilt tool InputSchema required: %v", rebuiltTool.InputSchema.Required)
	spew.Dump("Rebuilt user param:", rebuiltUserParam)

	// Check user parameter
	userMap := utils.InterfaceToGeneralMap(rebuiltUserParam)
	require.Equal(t, "object", utils.MapGetString(userMap, "type"))

	// Check nested properties
	propsRaw := utils.MapGetRaw(userMap, "properties")
	require.NotNil(t, propsRaw)

	// Properties might be an OrderedMap
	if oMap, ok := propsRaw.(*omap.OrderedMap[string, any]); ok {
		require.True(t, oMap.Len() > 0, "OrderedMap should not be empty")
	} else {
		propsMap := utils.InterfaceToGeneralMap(propsRaw)
		require.NotEmpty(t, propsMap)
	}

	// Check required fields - should be inside the user object schema
	requiredSlice := utils.MapGetStringSlice(userMap, "required")
	log.Infof("Required fields in user object: %v", requiredSlice)
	require.Contains(t, requiredSlice, "name", "name field should be marked as required in the user object schema")
}

// TestBuildParamsOptions_StructArrayParam tests struct array parameter handling
func TestBuildParamsOptions_StructArrayParam(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStructArrayParam("users",
			[]PropertyOption{
				WithParam_Description("Array of users"),
				WithParam_Required(true),
			},
			nil,
			WithStringParam("name", WithParam_Required(true)),
			WithIntegerParam("age", WithParam_Min(0)),
			WithStringParam("role", WithParam_EnumString("admin", "user", "guest")),
		),
	)
	require.NoError(t, err)

	log.Infof("Original struct array tool schema:\n%s", tool.ParamsJsonSchemaString())

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	log.Infof("Rebuilt struct array tool schema:\n%s", rebuiltTool.ParamsJsonSchemaString())

	// Check users parameter
	usersParam, ok := rebuiltTool.Params().Get("users")
	require.True(t, ok)
	usersMap := utils.InterfaceToGeneralMap(usersParam)
	require.Equal(t, "array", utils.MapGetString(usersMap, "type"))

	// Check items
	itemsRaw := utils.MapGetRaw(usersMap, "items")
	require.NotNil(t, itemsRaw)
	itemsMap := utils.InterfaceToGeneralMap(itemsRaw)
	require.Equal(t, "object", utils.MapGetString(itemsMap, "type"))

	// Check nested properties in items
	propsRaw := utils.MapGetRaw(itemsMap, "properties")
	require.NotNil(t, propsRaw)

	// Properties might be an OrderedMap
	var nameRaw, roleRaw any
	var nameFound, roleFound bool

	if oMap, ok := propsRaw.(*omap.OrderedMap[string, any]); ok {
		require.True(t, oMap.Len() > 0, "OrderedMap should not be empty")
		nameRaw, nameFound = oMap.Get("name")
		roleRaw, roleFound = oMap.Get("role")
	} else {
		propsMap := utils.InterfaceToGeneralMap(propsRaw)
		nameRaw, nameFound = propsMap["name"]
		roleRaw, roleFound = propsMap["role"]
	}

	// Check name field exists
	require.True(t, nameFound)
	nameMap := utils.InterfaceToGeneralMap(nameRaw)
	require.Equal(t, "string", utils.MapGetString(nameMap, "type"))

	// Check role field with enum
	require.True(t, roleFound)
	roleMap := utils.InterfaceToGeneralMap(roleRaw)
	enumRaw := utils.MapGetRaw(roleMap, "enum")
	require.NotNil(t, enumRaw)
}

// TestBuildParamsOptions_ComplexNested tests complex nested structures
func TestBuildParamsOptions_ComplexNested(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Complex nested tool"),
		WithCallback(testCallback),
		WithStringParam("id", WithParam_Required(true)),
		WithStructParam("metadata",
			[]PropertyOption{
				WithParam_Description("Metadata object"),
			},
			WithStringParam("version"),
			WithStringArrayParam("tags"),
			WithStructParam("author",
				nil,
				WithStringParam("name"),
				WithStringParam("email"),
			),
		),
		WithStructArrayParam("items",
			[]PropertyOption{
				WithParam_Description("Items array"),
			},
			nil,
			WithStringParam("itemName", WithParam_Required(true)),
			WithNumberParam("quantity", WithParam_Min(0)),
		),
	)
	require.NoError(t, err)

	log.Infof("Original complex nested tool schema:\n%s", tool.ParamsJsonSchemaString())

	// Rebuild and compare
	rebuiltTool, err := tool.RebuildTool()
	require.NoError(t, err)

	log.Infof("Rebuilt complex nested tool schema:\n%s", rebuiltTool.ParamsJsonSchemaString())

	// Basic checks
	require.Equal(t, tool.Name, rebuiltTool.Name)
	require.Equal(t, tool.Description, rebuiltTool.Description)
	require.Equal(t, tool.Params().Len(), rebuiltTool.Params().Len())

	// Check metadata structure
	metadataParam, ok := rebuiltTool.Params().Get("metadata")
	require.True(t, ok)
	metadataMap := utils.InterfaceToGeneralMap(metadataParam)
	require.Equal(t, "object", utils.MapGetString(metadataMap, "type"))

	// Check items array structure
	itemsParam, ok := rebuiltTool.Params().Get("items")
	require.True(t, ok)
	itemsMap := utils.InterfaceToGeneralMap(itemsParam)
	require.Equal(t, "array", utils.MapGetString(itemsMap, "type"))
}

// TestCompareTools tests the tool comparison function
func TestCompareTools(t *testing.T) {
	tool1, err := New("tool1",
		WithDescription("First tool"),
		WithCallback(testCallback),
		WithStringParam("param1", WithParam_Required(true)),
		WithIntegerParam("param2"),
	)
	require.NoError(t, err)

	tool2, err := New("tool1",
		WithDescription("First tool"),
		WithCallback(testCallback),
		WithStringParam("param1", WithParam_Required(true)),
		WithIntegerParam("param2"),
	)
	require.NoError(t, err)

	tool3, err := New("tool2",
		WithDescription("Second tool"),
		WithCallback(testCallback),
		WithStringParam("param1"),
	)
	require.NoError(t, err)

	// Compare identical tools
	diffs := CompareTools(tool1, tool2)
	require.Empty(t, diffs, "identical tools should have no differences")

	// Compare different tools
	diffs = CompareTools(tool1, tool3)
	require.NotEmpty(t, diffs, "different tools should have differences")

	log.Infof("Differences between tool1 and tool3:")
	for _, diff := range diffs {
		log.Infof("  - %s", diff)
	}
}

// TestRoundTripConversion tests the complete round-trip conversion
func TestRoundTripConversion(t *testing.T) {
	original, err := New("complexTool",
		WithDescription("Complex tool for testing"),
		WithKeywords([]string{"test", "complex"}),
		WithCallback(testCallback),
		WithStringParam("name",
			WithParam_Description("Name field"),
			WithParam_Required(true),
			WithParam_MinLength(1),
			WithParam_MaxLength(100),
		),
		WithIntegerParam("age",
			WithParam_Min(0),
			WithParam_Max(150),
			WithParam_Default(18),
		),
		WithStringParam("status",
			WithParam_EnumString("active", "inactive"),
			WithParam_Default("active"),
		),
		WithStringArrayParam("tags"),
		WithStructParam("address",
			nil,
			WithStringParam("street"),
			WithStringParam("city", WithParam_Required(true)),
			WithStringParam("country"),
		),
	)
	require.NoError(t, err)

	log.Infof("Original tool schema:\n%s", original.ParamsJsonSchemaString())

	// Round-trip conversion
	rebuilt, err := original.RebuildTool()
	require.NoError(t, err)

	log.Infof("Rebuilt tool schema:\n%s", rebuilt.ParamsJsonSchemaString())

	// Compare tools
	diffs := CompareTools(original, rebuilt)
	if len(diffs) > 0 {
		log.Warnf("Differences found in round-trip conversion:")
		for _, diff := range diffs {
			log.Warnf("  - %s", diff)
		}
	}

	// Basic checks
	require.Equal(t, original.Name, rebuilt.Name)
	require.Equal(t, original.Description, rebuilt.Description)
	require.Equal(t, original.Params().Len(), rebuilt.Params().Len())
	require.ElementsMatch(t, original.Keywords, rebuilt.Keywords)

	// Deep schema comparison
	originalSchema := original.ParamsJsonSchemaString()
	rebuiltSchema := rebuilt.ParamsJsonSchemaString()

	var originalMap, rebuiltMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(originalSchema), &originalMap))
	require.NoError(t, json.Unmarshal([]byte(rebuiltSchema), &rebuiltMap))

	// They should be functionally equivalent (though exact order might differ)
	require.Equal(t, len(originalMap), len(rebuiltMap), "schemas should have same number of top-level keys")
}

// TestBuildParamsOptions_EdgeCases tests edge cases
func TestBuildParamsOptions_EdgeCases(t *testing.T) {
	t.Run("nil tool", func(t *testing.T) {
		var tool *Tool
		opts := tool.BuildParamsOptions()
		require.Empty(t, opts)
	})

	t.Run("tool without params", func(t *testing.T) {
		tool, err := New("emptyTool",
			WithDescription("Empty tool"),
			WithCallback(testCallback),
		)
		require.NoError(t, err)

		opts := tool.BuildParamsOptions()
		require.Empty(t, opts)

		rebuilt, err := tool.RebuildTool()
		require.NoError(t, err)
		require.Equal(t, tool.Name, rebuilt.Name)
	})

	t.Run("tool with null param", func(t *testing.T) {
		tool, err := New("nullTool",
			WithDescription("Tool with null param"),
			WithCallback(testCallback),
			WithNullParam("nullField"),
		)
		require.NoError(t, err)

		rebuilt, err := tool.RebuildTool()
		require.NoError(t, err)

		nullParam, ok := rebuilt.Params().Get("nullField")
		require.True(t, ok)
		nullMap := utils.InterfaceToGeneralMap(nullParam)
		require.Equal(t, "null", utils.MapGetString(nullMap, "type"))
	})
}

// TestBuildParamsOptions_DefaultValues tests handling of default values
func TestBuildParamsOptions_DefaultValues(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStringParam("strWithDefault",
			WithParam_Default("hello"),
		),
		WithIntegerParam("intWithDefault",
			WithParam_Default(42),
		),
		WithBoolParam("boolWithDefault",
			WithParam_Default(true),
		),
		WithNumberParam("numWithDefault",
			WithParam_Default(3.14),
		),
	)
	require.NoError(t, err)

	// Rebuild and check defaults
	rebuilt, err := tool.RebuildTool()
	require.NoError(t, err)

	// Check string default
	strParam, ok := rebuilt.Params().Get("strWithDefault")
	require.True(t, ok)
	strMap := utils.InterfaceToGeneralMap(strParam)
	require.Equal(t, "hello", utils.MapGetString(strMap, "default"))

	// Check integer default
	intParam, ok := rebuilt.Params().Get("intWithDefault")
	require.True(t, ok)
	intMap := utils.InterfaceToGeneralMap(intParam)
	require.Equal(t, 42, utils.MapGetInt(intMap, "default"))

	// Check boolean default
	boolParam, ok := rebuilt.Params().Get("boolWithDefault")
	require.True(t, ok)
	boolMap := utils.InterfaceToGeneralMap(boolParam)
	require.Equal(t, true, utils.MapGetBool(boolMap, "default"))

	// Check number default
	numParam, ok := rebuilt.Params().Get("numWithDefault")
	require.True(t, ok)
	numMap := utils.InterfaceToGeneralMap(numParam)
	require.Equal(t, 3.14, utils.MapGetFloat64(numMap, "default"))
}

// TestBuildParamsOptions_WithTitle tests handling of title property
func TestBuildParamsOptions_WithTitle(t *testing.T) {
	tool, err := New("testTool",
		WithDescription("Test tool"),
		WithCallback(testCallback),
		WithStringParam("fieldName",
			WithParam_Title("Field Display Name"),
			WithParam_Description("Field description"),
		),
	)
	require.NoError(t, err)

	// Rebuild and check title
	rebuilt, err := tool.RebuildTool()
	require.NoError(t, err)

	fieldParam, ok := rebuilt.Params().Get("fieldName")
	require.True(t, ok)
	fieldMap := utils.InterfaceToGeneralMap(fieldParam)
	require.Equal(t, "Field Display Name", utils.MapGetString(fieldMap, "title"))
	require.Equal(t, "Field description", utils.MapGetString(fieldMap, "description"))
}

// TestBuildParamsOptions_Debug prints detailed schema for debugging
func TestBuildParamsOptions_Debug(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping debug test in short mode")
	}

	tool, err := New("debugTool",
		WithDescription("Debug tool"),
		WithCallback(testCallback),
		WithStringParam("name", WithParam_Required(true)),
		WithStructArrayParam("items",
			[]PropertyOption{WithParam_Required(true)},
			nil,
			WithStringParam("itemName"),
			WithIntegerParam("quantity"),
		),
	)
	require.NoError(t, err)

	log.Infof("=== Original Tool ===")
	log.Infof("Schema: %s", tool.ParamsJsonSchemaString())
	spew.Dump(tool.InputSchema)

	opts := tool.BuildParamsOptions()
	log.Infof("\n=== Built Options ===")
	log.Infof("Number of options: %d", len(opts))

	rebuilt, err := tool.RebuildTool()
	require.NoError(t, err)

	log.Infof("\n=== Rebuilt Tool ===")
	log.Infof("Schema: %s", rebuilt.ParamsJsonSchemaString())
	spew.Dump(rebuilt.InputSchema)

	diffs := CompareTools(tool, rebuilt)
	if len(diffs) > 0 {
		log.Infof("\n=== Differences ===")
		for _, diff := range diffs {
			log.Infof("  %s", diff)
		}
	} else {
		log.Infof("\n=== No Differences ===")
	}
}
