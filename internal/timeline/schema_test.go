package timeline

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const schemaPath = "../../skill/porcelain/timeline.schema.json"

type schemaNode struct {
	Ref        string                 `json:"$ref"`
	Type       string                 `json:"type"`
	Required   []string               `json:"required"`
	Properties map[string]*schemaNode `json:"properties"`
	Items      *schemaNode            `json:"items"`
	Defs       map[string]*schemaNode `json:"$defs"`
}

// TestSchemaInSync checks that every Go struct field has a schema property
// with matching required-ness (no omitempty = required), and vice versa.
func TestSchemaInSync(t *testing.T) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var root schemaNode
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	compare(t, "$", reflect.TypeFor[Timeline](), &root, root.Defs)
}

func compare(t *testing.T, at string, typ reflect.Type, n *schemaNode, defs map[string]*schemaNode) {
	t.Helper()
	if n.Ref != "" {
		n = defs[strings.TrimPrefix(n.Ref, "#/$defs/")]
	}
	switch typ.Kind() {
	case reflect.Slice:
		if n.Items == nil {
			t.Errorf("%s: schema has no items", at)
			return
		}
		compare(t, at+"[]", typ.Elem(), n.Items, defs)
		return
	case reflect.Struct:
	default:
		return
	}
	var required []string
	seen := map[string]bool{}
	for f := range typ.Fields() {
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		seen[name] = true
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
		p, ok := n.Properties[name]
		if !ok {
			t.Errorf("%s.%s: in Go, missing from schema", at, name)
			continue
		}
		compare(t, at+"."+name, f.Type, p, defs)
	}
	for name := range n.Properties {
		if !seen[name] {
			t.Errorf("%s.%s: in schema, missing from Go", at, name)
		}
	}
	slices.Sort(required)
	got := slices.Clone(n.Required)
	slices.Sort(got)
	if !slices.Equal(required, got) {
		t.Errorf("%s: required = %v in schema, want %v (fields without omitempty)", at, got, required)
	}
}
