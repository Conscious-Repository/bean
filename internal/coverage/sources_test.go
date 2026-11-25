package coverage

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// names is kept by hand. It must hold each member that a selection and a
// guide have, and no other: a key is tested against it.
func TestNamesAreTheMembers(t *testing.T) {
	var tags []string
	for _, v := range []any{Selection{}, Pin{}, guide{}, entry{}} {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			if tag != "" && tag != "-" && !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	slices.Sort(tags)
	want := slices.Sorted(slices.Values(names))
	if !slices.Equal(tags, want) {
		t.Errorf("the members are %v, and names holds %v", tags, want)
	}
}

// A key that folds to a member's name, and is not that name, is refused at
// any depth. Another key is not.
func TestExactKeys(t *testing.T) {
	for key, refused := range map[string]bool{"sha256": false, "SHA256": true, "ſha256": true, "pacKs": true, "Interpretation_Limit": true, "note": false, "sha": false} {
		tree := map[string]any{"a": []any{map[string]any{key: "x"}}}
		if err := exactKeys(tree); (err != nil) != refused {
			t.Errorf("the key %q: %v", key, err)
		}
	}
}
