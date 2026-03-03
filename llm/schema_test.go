package llm_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestSchemaOf_Constraints(t *testing.T) {
	type Described struct {
		Name string `json:"name" jsonschema:"description=The user name"`
	}
	type NumericBounds struct {
		Age int `json:"age" jsonschema:"minimum=0,maximum=150"`
	}
	type WithEnum struct {
		Color string `json:"color" jsonschema:"enum=red|green|blue"`
	}
	type WithPattern struct {
		Email string `json:"email" jsonschema:"pattern=^[a-z]+@[a-z]+$"`
	}
	type StringLength struct {
		Code string `json:"code" jsonschema:"minLength=2,maxLength=10"`
	}
	type ArrayBounds struct {
		Tags []string `json:"tags" jsonschema:"minItems=1,maxItems=5"`
	}
	type Multi struct {
		Score float64 `json:"score" jsonschema:"description=Rating,minimum=0,maximum=100"`
	}
	type Plain struct {
		Value string `json:"value"`
	}

	tests := []struct {
		name   string
		input  any
		field  string
		checks map[string]any
	}{
		{
			name: "description", input: Described{}, field: "name",
			checks: map[string]any{"description": "The user name"},
		},
		{
			name: "numeric_min_max", input: NumericBounds{}, field: "age",
			checks: map[string]any{"minimum": float64(0), "maximum": float64(150)},
		},
		{
			name: "enum", input: WithEnum{}, field: "color",
			checks: map[string]any{"enum": []string{"red", "green", "blue"}},
		},
		{
			name: "pattern", input: WithPattern{}, field: "email",
			checks: map[string]any{"pattern": "^[a-z]+@[a-z]+$"},
		},
		{
			name: "string_length", input: StringLength{}, field: "code",
			checks: map[string]any{"minLength": float64(2), "maxLength": float64(10)},
		},
		{
			name: "array_bounds", input: ArrayBounds{}, field: "tags",
			checks: map[string]any{"minItems": float64(1), "maxItems": float64(5)},
		},
		{
			name: "multiple_constraints", input: Multi{}, field: "score",
			checks: map[string]any{
				"description": "Rating",
				"minimum":     float64(0),
				"maximum":     float64(100),
			},
		},
		{
			name: "no_tag_unchanged", input: Plain{}, field: "value",
			checks: map[string]any{"type": "string"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schema := llm.SchemaOf(tc.input)
			props := schema["properties"].(map[string]any)
			fs := props[tc.field].(map[string]any)
			for k, want := range tc.checks {
				got, ok := fs[k]
				if !ok {
					t.Errorf("missing key %q in schema", k)
					continue
				}
				if wantSlice, isSlice := want.([]string); isSlice {
					gotSlice, ok := got.([]string)
					if !ok {
						t.Errorf("key %q: got type %T, want []string", k, got)
						continue
					}
					if len(gotSlice) != len(wantSlice) {
						t.Errorf("key %q: got %v, want %v", k, gotSlice, wantSlice)
						continue
					}
					for i := range wantSlice {
						if gotSlice[i] != wantSlice[i] {
							t.Errorf("key %q[%d]: got %q, want %q", k, i, gotSlice[i], wantSlice[i])
						}
					}
				} else if got != want {
					t.Errorf("key %q: got %v, want %v", k, got, want)
				}
			}
		})
	}
}

func TestSchemaOf_InvalidNumericSkipped(t *testing.T) {
	type Bad struct {
		X int `json:"x" jsonschema:"minimum=abc"`
	}
	schema := llm.SchemaOf(Bad{})
	props := schema["properties"].(map[string]any)
	fs := props["x"].(map[string]any)
	if _, ok := fs["minimum"]; ok {
		t.Error("invalid numeric constraint should be skipped, but minimum is present")
	}
}

func TestSchemaOf_EmptyTagNoOp(t *testing.T) {
	type Empty struct {
		Y string `json:"y" jsonschema:""`
	}
	schema := llm.SchemaOf(Empty{})
	props := schema["properties"].(map[string]any)
	fs := props["y"].(map[string]any)
	if len(fs) != 1 {
		t.Errorf("empty tag should not add keys, got %v", fs)
	}
	if fs["type"] != "string" {
		t.Errorf("type = %v, want string", fs["type"])
	}
}
