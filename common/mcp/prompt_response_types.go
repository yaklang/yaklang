package mcp

import "encoding/json"

type baseGetPromptRequestParamsArguments struct {
	// We will deserialize the arguments into the users struct later on
	Arguments json.RawMessage `json:"arguments,omitempty" yaml:"arguments,omitempty" mapstructure:"arguments,omitempty"`

	// The name of the prompt or prompt template.
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// The server's response to a prompts/list request from the client.
type ListPromptsResponse struct {
	// Prompts corresponds to the JSON schema field "prompts".
	Prompts []*PromptSchema `json:"prompts" yaml:"prompts" mapstructure:"prompts"`
	// NextCursor is a cursor for pagination. If not nil, there are more prompts available.
	NextCursor *string `json:"nextCursor,omitempty" yaml:"nextCursor,omitempty" mapstructure:"nextCursor,omitempty"`
}

// A PromptSchema or prompt template that the server offers.
type PromptSchema struct {
	// A list of arguments to use for templating the prompt.
	Arguments []PromptSchemaArgument `json:"arguments,omitempty" yaml:"arguments,omitempty" mapstructure:"arguments,omitempty"`

	// An optional description of what this prompt provides
	Description *string `json:"description,omitempty" yaml:"description,omitempty" mapstructure:"description,omitempty"`

	// The name of the prompt or prompt template.
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

type PromptSchemaArgument struct {
	// A human-readable description of the argument.
	Description *string `json:"description,omitempty" yaml:"description,omitempty" mapstructure:"description,omitempty"`

	// The name of the argument.
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Whether this argument must be provided.
	Required *bool `json:"required,omitempty" yaml:"required,omitempty" mapstructure:"required,omitempty"`
}
