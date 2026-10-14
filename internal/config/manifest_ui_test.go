package config

import (
	"encoding/json"
	"os"
	"sort"
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

// Every size setting is declared with a title, a default of 0 (inherit) and a
// description, and sits in the "Sizes" group of the form.
func TestSizeSettingsAreDeclaredAndGrouped(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
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
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	inGroup := map[string]bool{}
	for _, g := range m.Config.UISchema.Elements {
		if g.Label == "Sizes" {
			for _, e := range g.Elements {
				inGroup[e.Scope] = true
			}
		}
	}
	want := []string{"sizeNewMusic", "sizeLoved", "sizeOnRepeat", "sizeEssentials", "sizeDiscovery",
		"sizeRediscover", "sizeWrapped", "sizeGenreRadio", "sizeArtistRadio", "sizeDailyMix",
		"sizeDecade", "sizeTimeOfDay", "lovedMaxPerArtist"}
	for _, k := range want {
		p, ok := m.Config.Schema.Properties[k]
		if !ok || p.Title == "" || p.Description == "" || p.Default != float64(0) {
			t.Errorf("%s: missing title, description or a default of 0", k)
		}
		wantMax := 200
		if k == "lovedMaxPerArtist" {
			wantMax = 1000
		}
		if p.Maximum != wantMax {
			t.Errorf("%s maximum = %d, want %d", k, p.Maximum, wantMax)
		}
		if !inGroup["#/properties/"+k] {
			t.Errorf("%s is not in the Sizes group", k)
		}
	}
}
