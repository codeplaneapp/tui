package smithers

// Prompt represents a discovered prompt from .smithers/prompts/.
// Maps to DiscoveredPrompt in smithers/src/cli/prompts.ts
type Prompt struct {
	// ID is the prompt identifier derived from the filename (without extension).
	// Example: "implement", "research", "review"
	ID string `json:"id"`

	// EntryFile is the relative path to the .mdx source file.
	// Example: ".smithers/prompts/implement.mdx"
	EntryFile string `json:"entryFile"`

	// Source is the full MDX source content of the prompt file.
	// Populated by GetPrompt; may be empty in list results.
	Source string `json:"source,omitempty"`

	// Props lists the interpolation variables discovered in the MDX source.
	// Example: [{Name: "prompt", Type: "string"}, {Name: "schema", Type: "string"}]
	Props []PromptProp `json:"inputs,omitempty"`
}

// PromptProp describes a single interpolation variable in a prompt template.
// Maps to the inputs[] entries in DiscoveredPrompt from smithers/src/cli/prompts.ts
type PromptProp struct {
	// Name is the variable name as used in {props.Name} expressions.
	Name string `json:"name"`

	// Type is the declared type (e.g. "string", "number", "boolean").
	// Defaults to "string" when not explicitly declared.
	Type string `json:"type"`

	// DefaultValue is the optional default, serialized as a string.
	DefaultValue *string `json:"defaultValue,omitempty"`
}
