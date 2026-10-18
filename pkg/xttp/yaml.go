package xttp

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
)

// marshalYAML writes the document's plain maps as block YAML with sorted keys.
// It covers only what Spec produces (maps, slices, scalars); strings are written
// JSON-quoted, which is valid YAML and leaves nothing to type-guess.
func marshalYAML(v any) []byte {
	var b strings.Builder
	writeYAML(&b, v, 0)
	return []byte(b.String())
}

var plainKey = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_.$/-]*$`)

func writeYAML(b *strings.Builder, v any, indent int) {
	pad := strings.Repeat(" ", indent)
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			b.WriteString(pad)
			if plainKey.MatchString(k) {
				b.WriteString(k)
			} else {
				b.WriteString(scalar(k))
			}
			b.WriteString(":")
			writeValue(b, v[k], indent)
		}
	case []any:
		for _, item := range v {
			b.WriteString(pad + "-")
			writeValue(b, item, indent)
		}
	}
}

// writeValue writes a value after "key:" or "-": inline when it is a scalar or
// empty, as an indented block on the next lines otherwise.
func writeValue(b *strings.Builder, v any, indent int) {
	switch c := v.(type) {
	case map[string]any:
		if len(c) == 0 {
			b.WriteString(" {}\n")
			return
		}
	case []any:
		if len(c) == 0 {
			b.WriteString(" []\n")
			return
		}
	default:
		b.WriteString(" " + scalar(v) + "\n")
		return
	}
	b.WriteString("\n")
	writeYAML(b, v, indent+2)
}

func scalar(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(out)
}
