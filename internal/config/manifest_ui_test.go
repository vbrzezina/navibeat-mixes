package config

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// Navidrome draws the plugin settings form from config.uiSchema, not from
// config.schema. A property that is in the schema but has no Control in the
// uiSchema is read by the plugin and never shown to anyone, so nobody can set
// it. That is how the 0.9.13 libraries setting shipped: in the schema, in the
// code, in the README, and absent from the form (navibeat-mixes#8).
func TestEverySettingHasAControlInTheForm(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Config struct {
			Schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schema"`
			UISchema json.RawMessage `json:"uiSchema"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var ui any
	if err := json.Unmarshal(m.Config.UISchema, &ui); err != nil {
		t.Fatal(err)
	}
	shown := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if s, ok := x["scope"].(string); ok {
				const p = "#/properties/"
				if len(s) > len(p) && s[:len(p)] == p {
					shown[s[len(p):]] = true
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(ui)
	var missing []string
	for name := range m.Config.Schema.Properties {
		if !shown[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("settings with no control in the form, so no user can set them: %v", missing)
	}
}

// manifestForm is the part of manifest.json the tests below read.
type manifestForm struct {
	Config struct {
		Schema struct {
			Properties map[string]struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Default     any    `json:"default"`
				Maximum     int    `json:"maximum"`
			} `json:"properties"`
		} `json:"schema"`
		UISchema struct {
			Elements []struct {
				Label    string `json:"label"`
				Elements []struct {
					Scope string `json:"scope"`
				} `json:"elements"`
			} `json:"elements"`
		} `json:"uiSchema"`
	} `json:"config"`
}

func readManifestForm(t *testing.T) (manifestForm, func(group string) map[string]bool) {
	t.Helper()
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifestForm
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, func(group string) map[string]bool {
		in := map[string]bool{}
		for _, g := range m.Config.UISchema.Elements {
			if g.Label == group {
				for _, e := range g.Elements {
					in[strings.TrimPrefix(e.Scope, "#/properties/")] = true
				}
			}
		}
		return in
	}
}

// Every size setting the CODE reads (sizeSettings, plus lovedMaxPerArtist) is
// declared with a title, a default of 0 (inherit) and a description, sits in
// the "Sizes" group, and the other way round: no size* property in the
// manifest is unknown to the code. Iterating the code's own table is the
// point: a hard coded list here would not notice a setting added to one side.
func TestSizeSettingsAreDeclaredAndGrouped(t *testing.T) {
	m, group := readManifestForm(t)
	inGroup := group("Sizes")
	want := map[string]int{"lovedMaxPerArtist": MaxLovedPerArtist}
	for _, f := range sizeSettings {
		want[f.field] = MaxMixSize
	}
	for k, max := range want {
		p, ok := m.Config.Schema.Properties[k]
		if !ok || p.Title == "" || p.Description == "" || p.Default != float64(0) {
			t.Errorf("%s: missing title, description or a default of 0", k)
			continue
		}
		if p.Maximum != max {
			t.Errorf("%s maximum = %d, want %d", k, p.Maximum, max)
		}
		if !inGroup[k] {
			t.Errorf("%s is not in the Sizes group", k)
		}
		if strings.HasPrefix(k, "size") && !strings.Contains(p.Description, "candidate pool") {
			t.Errorf("%s: description does not carry the candidate pool caveat", k)
		}
	}
	for k := range m.Config.Schema.Properties {
		if strings.HasPrefix(k, "size") {
			if _, ok := want[k]; !ok {
				t.Errorf("%s is in the manifest but not in the code's sizeSettings", k)
			}
		}
	}
	for k := range inGroup {
		if _, ok := want[k]; !ok {
			t.Errorf("%s is in the Sizes group but not read by the code", k)
		}
	}
}

// Every name template the code reads (templateFamilies) and every slot name
// (nameSlots) is declared and in the "Names" group, in both directions. The
// templates also have to say what the user needs to know: the placeholder,
// the 100 character cut and the collision fallback.
func TestNameSettingsAreDeclaredAndGrouped(t *testing.T) {
	m, group := readManifestForm(t)
	inGroup := group("Names")
	want := map[string]bool{}
	for _, s := range nameSlots {
		want["name"+s.suffix] = false
	}
	for _, f := range templateFamilies {
		want[f.field] = true
	}
	for k, isTemplate := range want {
		p, ok := m.Config.Schema.Properties[k]
		if !ok || p.Title == "" {
			t.Errorf("%s: missing from the manifest or has no title", k)
			continue
		}
		if !inGroup[k] {
			t.Errorf("%s is not in the Names group", k)
		}
		if !isTemplate {
			continue
		}
		if p.Default != "" || p.Description == "" {
			t.Errorf("%s: want an empty default and a description", k)
		}
		for _, need := range []string{"{subject}", "100 characters", "numbered default"} {
			if !strings.Contains(p.Description, need) {
				t.Errorf("%s: description lacks %q", k, need)
			}
		}
	}
	for k := range m.Config.Schema.Properties {
		if strings.HasPrefix(k, "name") {
			if _, ok := want[k]; !ok {
				t.Errorf("%s is in the manifest but not in the code's name tables", k)
			}
		}
	}
	for k := range inGroup {
		if _, ok := want[k]; !ok {
			t.Errorf("%s is in the Names group but not read by the code", k)
		}
	}
}
