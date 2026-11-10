package aiforge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/yakcliconvert"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/information"
)

var subForgeAlias = regexp.MustCompile(`^[a-z][a-z0-9_]{0,53}$`)

type subForgeBinding struct {
	alias       string
	description string
	child       *ForgeBlueprint
}

// WithSubForge offers a child blueprint to the parent's execution workers as
// sub_forge_<alias>. The description explains when the model should call it.
func WithSubForge(alias, description string, childBlueprint *ForgeBlueprint) Option {
	return func(f *ForgeBlueprint) {
		f.subForges = append(f.subForges, subForgeBinding{alias, description, childBlueprint})
	}
}

func (f *ForgeBlueprint) prepareSubForges(ctx context.Context) ([]coordinator.SubForgeTool, error) {
	if len(f.subForges) == 0 || coordinator.IsSubForgeContext(ctx) {
		return nil, nil
	}
	seen := make(map[string]bool, len(f.subForges)+len(f.Tools))
	for _, tool := range f.Tools {
		if tool != nil {
			seen[tool.Name] = true
		}
	}
	bindings := make([]coordinator.SubForgeTool, 0, len(f.subForges))
	for _, binding := range f.subForges {
		name := "sub_forge_" + binding.alias
		child := binding.child
		if !subForgeAlias.MatchString(binding.alias) || strings.TrimSpace(binding.description) == "" {
			return nil, fmt.Errorf("sub-Forge %q requires a valid alias and description", binding.alias)
		}
		if child == nil || strings.TrimSpace(child.Name) == "" || child == f || child.Name == f.Name {
			return nil, fmt.Errorf("sub-Forge %q requires a named child blueprint distinct from %q", binding.alias, f.Name)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate sub-Forge tool name %q", name)
		}
		seen[name] = true
		tool, err := subForgeSchema(name, binding.description, child.ParameterRuleYaklangCode)
		if err != nil {
			return nil, fmt.Errorf("sub-Forge %q: %w", binding.alias, err)
		}
		childBlueprint := *child
		bindings = append(bindings, coordinator.SubForgeTool{
			Schema: tool,
			Tools:  append([]*aitool.Tool(nil), child.Tools...),
			Run: func(ctx context.Context, params aitool.InvokeParams, allowed []*aitool.Tool, opts ...aicommon.ConfigOption) (any, error) {
				query, arguments, err := subForgeArguments(tool, params)
				if err != nil {
					return nil, err
				}
				blueprint := childBlueprint
				blueprint.Tools = allowed
				execution, err := blueprint.CreateCoordinatorWithQueryAndParams(ctx, query, arguments, opts...)
				if err != nil {
					return nil, err
				}
				defer execution.Close()
				if err := execution.Run(); err != nil {
					return nil, err
				}
				result := execution.Result()
				if result.Action != nil {
					return result.Action.GetParams(), nil
				}
				return result.Formated, nil
			},
		})
	}
	return bindings, nil
}

func subForgeSchema(name, description, cli string) (*mcp.Tool, error) {
	if strings.TrimSpace(cli) == "" {
		return mcp.NewTool(name, mcp.WithDescription(description), mcp.WithString("query", mcp.Required())), nil
	}
	program, err := static_analyzer.SSAParse(cli, "yak")
	if err != nil {
		return nil, fmt.Errorf("parse CLI parameter declarations: %w", err)
	}
	parameters, _, _ := information.ParseCliParameter(program)
	tool, err := yakcliconvert.ConvertCliParameterToToolChecked(name, program)
	if err != nil {
		return nil, err
	}
	tool.Description = description
	if len(parameters) == 0 && tool.InputSchema.Properties.Len() == 0 {
		return mcp.NewTool(name, mcp.WithDescription(description), mcp.WithString("query", mcp.Required())), nil
	}
	if len(parameters) != tool.InputSchema.Properties.Len() {
		return nil, fmt.Errorf("CLI declarations must map to distinct tool parameters")
	}
	for _, parameter := range parameters {
		if parameter.Name == "runtime_id" || parameter.Name == "@action" {
			return nil, fmt.Errorf("reserved CLI parameter %q", parameter.Name)
		}
		if _, ok := tool.InputSchema.Properties.Get(parameter.Name); !ok {
			return nil, fmt.Errorf("unsupported CLI parameter %q", parameter.Name)
		}
	}
	// Compile once at binding time so an invalid schema never reaches the model.
	raw, err := json.Marshal(tool.InputSchema.ToMap())
	if err != nil {
		return nil, err
	}
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("sub-forge.json", schema); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile("sub-forge.json")
	if err != nil {
		return nil, fmt.Errorf("invalid tool schema: %w", err)
	}
	for name, property := range compiled.Properties {
		if property.Default != nil {
			if err := property.Validate(*property.Default); err != nil {
				return nil, fmt.Errorf("invalid default for %q: %w", name, err)
			}
		}
	}
	return tool, nil
}

func subForgeArguments(tool *mcp.Tool, params aitool.InvokeParams) (string, []Parameter, error) {
	if params == nil {
		params = make(aitool.InvokeParams)
	}
	validator := &aitool.Tool{Tool: tool}
	if valid, issues := validator.ValidateParams(params); !valid {
		return "", nil, fmt.Errorf("invalid sub-Forge arguments: %s", strings.Join(issues, "; "))
	}
	for name := range params {
		if _, exists := tool.InputSchema.Properties.Get(name); !exists {
			return "", nil, fmt.Errorf("unknown sub-Forge parameter %q", name)
		}
	}
	var query string
	var encodeErr error
	args := make([]Parameter, 0, len(params))
	tool.InputSchema.Properties.ForEach(func(name string, _ any) bool {
		value, exists := params[name]
		if !exists {
			return true
		}
		text, ok := value.(string)
		if !ok {
			var raw []byte
			raw, encodeErr = json.Marshal(value)
			if encodeErr != nil {
				return false
			}
			text = string(raw)
		}
		if name == "query" {
			query = text
		}
		args = append(args, Parameter{Key: name, Value: text})
		return true
	})
	return query, args, encodeErr
}
