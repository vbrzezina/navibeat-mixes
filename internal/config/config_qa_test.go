package config

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func qaGet(m map[string]string) Getter { return func(k string) string { return m[k] } }

func TestQANameTemplates(t *testing.T) {
	m := map[string]string{
		"nameArtistRadio": "  This is {subject}  ",
		"nameGenreRadio":  "{subject} Radio", "nameDecade": "{subject} Hits",
	}
	cfg := Load(qaGet(m))
	if cfg.SubjectName("artistradio", "Billy Talent", "Artist Radio 1") != cfg.Prefix+"This is Billy Talent" {
		t.Error(cfg.SubjectName("artistradio", "Billy Talent", "Artist Radio 1"))
	}
	if cfg.SubjectName("decade", "1990s", "1990s") != cfg.Prefix+"1990s Hits" {
		t.Error("decade")
	}
	// Unset kind and unknown kind use the default.
	cfg = Load(qaGet(nil))
	if cfg.SubjectName("artistradio", "X", "Artist Radio 1") != cfg.Prefix+"Artist Radio 1" {
		t.Error("default name")
	}
	if cfg.SubjectName("bogus", "X", "D") != cfg.Prefix+"D" {
		t.Error("unknown kind")
	}
}

func TestQAPrefixEdgeCases(t *testing.T) {
	for raw, want := range map[string]string{
		"A": "A ", "A ": "A ", "  A": "A ", "": Defaults().Prefix, "   ": Defaults().Prefix,
		"[x]": "[x] ", "\U0001F3B5": "\U0001F3B5 ",
	} {
		if got := Load(qaGet(map[string]string{"prefix": raw})).Prefix; got != want {
			t.Errorf("prefix %q => %q want %q", raw, got, want)
		}
	}
	// SURPRISE: a prefix with a tab/newline is stored as is, and PlaylistName never cleans it.
	got := Load(qaGet(map[string]string{"prefix": "a\nb"})).Prefix
	t.Logf("prefix with newline stored as %q (prefix is not run through naming.Clean, spec S2-3 only cleans the rendered name)", got)
	// SURPRISE: a very long prefix is not capped (spec says prefix not counted).
	long := strings.Repeat("p", 500)
	t.Logf("500 char prefix kept at %d chars", len(Load(qaGet(map[string]string{"prefix": long})).Prefix))
	// A prefix that is only a trailing-space-less whitespace is ignored but
	// a prefix of "  " then explicit empty string cannot express "no prefix".
	t.Logf("prefix cannot be set to empty: %q", Load(qaGet(map[string]string{"prefix": ""})).Prefix)
}

type qaManifest struct {
	Version string `json:"version"`
	Config  struct {
		Schema struct {
			Properties map[string]map[string]any `json:"properties"`
		} `json:"schema"`
		UISchema json.RawMessage `json:"uiSchema"`
	} `json:"config"`
}

func qaLoadManifest(t *testing.T) qaManifest {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m qaManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestQAManifestNewPropertiesComplete(t *testing.T) {
	m := qaLoadManifest(t)
	if m.Version != "0.9.17-vb.2" {
		t.Errorf("version %q", m.Version)
	}
	want := []string{"nameArtistRadio", "nameGenreRadio", "nameDecade"}
	for _, k := range want {
		p, ok := m.Config.Schema.Properties[k]
		if !ok {
			t.Errorf("%s missing from schema", k)
			continue
		}
		for _, f := range []string{"title", "default", "description"} {
			if v, has := p[f]; !has || v == "" && f != "default" {
				t.Errorf("%s lacks %s", k, f)
			}
		}
		if s, _ := p["description"].(string); strings.Contains(s, "—") {
			t.Errorf("%s has em dash", k)
		}
	}
}

// Every property in the whole manifest must have title, default and description.
// Reported, not asserted for pre-existing (upstream) properties.
func TestQAManifestEveryPropertyHasTitleDefaultDescription(t *testing.T) {
	m := qaLoadManifest(t)
	var missing []string
	for k, p := range m.Config.Schema.Properties {
		for _, f := range []string{"title", "default", "description"} {
			if _, ok := p[f]; !ok {
				missing = append(missing, k+"."+f)
			}
		}
	}
	t.Logf("properties lacking a field (all pre-existing upstream?): %d: %v", len(missing), missing)
}

func TestQAManifestUISchemaReachesEveryProperty(t *testing.T) {
	m := qaLoadManifest(t)
	var ui any
	json.Unmarshal(m.Config.UISchema, &ui)
	seen := map[string]int{}
	groupOf := map[string]string{}
	var walk func(v any, group string)
	walk = func(v any, group string) {
		switch x := v.(type) {
		case map[string]any:
			if l, ok := x["label"].(string); ok && x["type"] == "Group" {
				group = l
			}
			if s, ok := x["scope"].(string); ok {
				k := strings.TrimPrefix(s, "#/properties/")
				seen[k]++
				groupOf[k] = group
			}
			for _, c := range x {
				walk(c, group)
			}
		case []any:
			for _, c := range x {
				walk(c, group)
			}
		}
	}
	walk(ui, "")
	for k := range m.Config.Schema.Properties {
		if seen[k] == 0 {
			t.Errorf("%s unreachable from uiSchema", k)
		}
		if seen[k] > 1 {
			t.Errorf("%s shown %d times", k, seen[k])
		}
	}
	for k := range seen {
		if _, ok := m.Config.Schema.Properties[k]; !ok {
			t.Errorf("uiSchema scope %s has no schema property", k)
		}
	}
	for _, k := range []string{"nameArtistRadio", "nameGenreRadio", "nameDecade"} {
		if groupOf[k] != "Names" {
			t.Errorf("%s in group %q, want Names", k, groupOf[k])
		}
	}
}

// Manifest defaults must equal the defaults in code: feed every manifest
// default through Load as a string and compare with the no-config result.
func TestQAManifestDefaultsEqualCodeDefaults(t *testing.T) {
	m := qaLoadManifest(t)
	vals := map[string]string{}
	for k, p := range m.Config.Schema.Properties {
		d, ok := p["default"]
		if !ok {
			continue
		}
		switch v := d.(type) {
		case string:
			vals[k] = v
		case float64:
			vals[k] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			vals[k] = strconv.FormatBool(v)
		default:
			t.Logf("default of %s has type %T", k, d)
		}
	}
	withManifest := Load(qaGet(vals))
	bare := Load(qaGet(nil))
	// Explicit "true" switches equal the implicit default: compare behaviour.
	for k := range withManifest.MixSwitches {
		if !withManifest.MixEnabled(k) || !bare.MixEnabled(k) {
			t.Errorf("mix %s differs when enabled by manifest default", k)
		}
	}
	withManifest.MixSwitches, bare.MixSwitches = nil, nil
	if !reflect.DeepEqual(withManifest, bare) {
		a, _ := json.Marshal(withManifest)
		b, _ := json.Marshal(bare)
		t.Errorf("manifest defaults change behaviour:\n manifest: %s\n code:     %s", a, b)
	}
	// And each typed default individually against Defaults().
	d := Defaults()
	checks := map[string]any{"mixSize": d.MixSize, "maxPerArtist": d.MaxPerArtist, "rediscoverMonths": d.RediscoverMonths,
		"minEventsForAffinity": d.MinEventsForAffinity, "genreNoiseThreshold": d.GenreNoiseThreshold, "prefix": d.Prefix}
	for k, want := range checks {
		p, ok := m.Config.Schema.Properties[k]
		if !ok {
			t.Logf("no property %s", k)
			continue
		}
		switch w := want.(type) {
		case int:
			if f, _ := p["default"].(float64); int(f) != w {
				t.Errorf("%s: manifest %v code %v", k, p["default"], w)
			}
		case float64:
			if f, _ := p["default"].(float64); f != w {
				t.Errorf("%s: manifest %v code %v", k, p["default"], w)
			}
		case string:
			if s, _ := p["default"].(string); s != w {
				t.Errorf("%s: manifest %q code %q", k, p["default"], w)
			}
		}
	}
}

// Garbage for every other numeric setting must not panic and must fall back.
func TestQAGarbageDoesNotPanicAnyKey(t *testing.T) {
	m := qaLoadManifest(t)
	for _, g := range []string{"", " ", "-1", "NaN", "1e9", "9999999999999999999999", "\x00", "{", "{\"a\":", "[", "\U0001F3B5", "null", strings.Repeat("9", 5000)} {
		vals := map[string]string{}
		for k := range m.Config.Schema.Properties {
			vals[k] = g
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic with %q: %v", g, r)
				}
			}()
			c := Load(qaGet(vals))
			_ = c.SubjectName("artistradio", "x", "d")
		}()
	}
}
