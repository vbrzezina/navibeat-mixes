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

var qaSizeFields = map[string]string{
	"sizeNewMusic": "newmusic", "sizeLoved": "loved", "sizeOnRepeat": "onrepeat",
	"sizeEssentials": "essentials", "sizeDiscovery": "discovery", "sizeRediscover": "rediscover",
	"sizeWrapped": "wrapped", "sizeGenreRadio": "genreradio", "sizeArtistRadio": "artistradio",
	"sizeDailyMix": "dailymix", "sizeDecade": "decade", "sizeTimeOfDay": "morning",
}

// S3-2 / S3-4 for every size setting.
func TestQASizeSettingsTable(t *testing.T) {
	cases := []struct {
		raw  string
		want int // 0 means inherit (mixSize 30 here)
	}{
		{"", 0}, {"0", 0}, {"1", 10}, {"9", 10}, {"10", 10}, {"11", 11}, {"30", 30},
		{"199", 199}, {"200", 200}, {"201", 0}, {"1000", 0}, {"-1", 0}, {"-0", 0},
		{" 25 ", 25}, {"25.0", 0}, {"25.5", 0}, {"1e2", 0}, {"1e9", 0}, {"NaN", 0},
		{"Inf", 0}, {"abc", 0}, {"  ", 0}, {"+25", 25}, {"0x20", 0}, {"025", 25},
		{"99999999999999999999", 0}, {"-99999999999999999999", 0}, {"2 5", 0}, {"١٢", 0},
		{"true", 0}, {"null", 0}, {"[]", 0},
	}
	for field, key := range qaSizeFields {
		for _, c := range cases {
			cfg := Load(qaGet(map[string]string{field: c.raw}))
			want := c.want
			if want == 0 {
				want = 30
			}
			if got := cfg.SizeFor(key); got != want {
				t.Errorf("%s=%q => SizeFor(%s)=%d want %d", field, c.raw, key, got, want)
			}
		}
	}
}

func TestQATimeOfDaySizeCoversAllFourSlots(t *testing.T) {
	cfg := Load(qaGet(map[string]string{"sizeTimeOfDay": "15", "mixSize": "40"}))
	for _, s := range []string{"morning", "afternoon", "evening", "night", "MORNING", " night "} {
		if got := cfg.SizeFor(s); got != 15 {
			t.Errorf("%q => %d", s, got)
		}
	}
	if cfg.SizeFor("loved") != 40 {
		t.Error("other mixes must inherit mixSize")
	}
}

func TestQAOverridesAreIndependent(t *testing.T) {
	cfg := Load(qaGet(map[string]string{"sizeLoved": "100", "mixSize": "20"}))
	for _, k := range []string{"newmusic", "onrepeat", "essentials", "discovery", "rediscover", "wrapped", "genreradio", "artistradio", "dailymix", "decade", "morning"} {
		if cfg.SizeFor(k) != 20 {
			t.Errorf("%s leaked override: %d", k, cfg.SizeFor(k))
		}
	}
	if cfg.SizeFor("loved") != 100 {
		t.Error("loved not 100")
	}
}

func TestQAUnknownKeyUsesMixSize(t *testing.T) {
	cfg := Load(qaGet(nil))
	if cfg.SizeFor("nonsense") != 30 || cfg.SizeFor("") != 30 {
		t.Error("unknown key")
	}
}

// SURPRISE probe: a per-mix size larger than mixSize's own bounds/ mixSize garbage.
func TestQAMixSizeItselfIsNotClamped(t *testing.T) {
	for _, raw := range []string{"5", "9", "201", "1000000", "-4", "abc", "1e9"} {
		cfg := Load(qaGet(map[string]string{"mixSize": raw}))
		t.Logf("mixSize=%q => %d (spec only bounds the new size* settings; mixSize min10/max200 exists only in the form schema)", raw, cfg.MixSize)
	}
}

func TestQALovedMaxPerArtist(t *testing.T) {
	cases := []struct {
		raw  string
		want int // 0 => inherit maxPerArtist (3)
	}{
		{"", 0}, {"0", 0}, {"1", 1}, {"5", 5}, {"1000", 1000}, {"1001", 0}, {"-1", 0},
		{"2.5", 0}, {"1e3", 0}, {"NaN", 0}, {"abc", 0}, {" 7 ", 7}, {"99999999999999999999", 0}, {"+9", 9},
	}
	for _, c := range cases {
		cfg := Load(qaGet(map[string]string{"lovedMaxPerArtist": c.raw}))
		want := c.want
		if want == 0 {
			want = 3
		}
		if got := cfg.MaxPerArtistFor("loved"); got != want {
			t.Errorf("lovedMaxPerArtist=%q => %d want %d", c.raw, got, want)
		}
		for _, k := range []string{"newmusic", "onrepeat", "essentials", "dailymix", "morning", "genreradio"} {
			if cfg.MaxPerArtistFor(k) != 3 {
				t.Errorf("lovedMaxPerArtist leaked into %s", k)
			}
		}
	}
	// replaces a customised maxPerArtist too
	cfg := Load(qaGet(map[string]string{"maxPerArtist": "7", "lovedMaxPerArtist": "2"}))
	if cfg.MaxPerArtistFor("loved") != 2 || cfg.MaxPerArtistFor("newmusic") != 7 {
		t.Error("replace semantics")
	}
	cfg = Load(qaGet(map[string]string{"maxPerArtist": "7"}))
	if cfg.MaxPerArtistFor("loved") != 7 {
		t.Error("inherit semantics")
	}
	if cfg.MaxPerArtistFor(" LOVED ") != 7 {
		t.Error("key normalisation")
	}
}

func TestQANameTemplates(t *testing.T) {
	m := map[string]string{
		"nameArtistRadio": "  This is {subject}  ", "nameDailyMix": "{subject} Mix",
		"nameGenreRadio": "{subject} Radio", "nameDecade": "{subject} Hits",
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
	if m.Version != "0.9.17-vb.1" {
		t.Errorf("version %q", m.Version)
	}
	var want []string
	for f := range qaSizeFields {
		want = append(want, f)
	}
	want = append(want, "lovedMaxPerArtist", "nameArtistRadio", "nameDailyMix", "nameGenreRadio", "nameDecade")
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
	for k := range qaSizeFields {
		if groupOf[k] != "Sizes" {
			t.Errorf("%s in group %q, want Sizes", k, groupOf[k])
		}
	}
	for _, k := range []string{"nameArtistRadio", "nameDailyMix", "nameGenreRadio", "nameDecade"} {
		if groupOf[k] != "Names" {
			t.Errorf("%s in group %q, want Names", k, groupOf[k])
		}
	}
	t.Logf("lovedMaxPerArtist is in group %q", groupOf["lovedMaxPerArtist"])
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
	for k, p := range m.Config.Schema.Properties {
		if strings.HasPrefix(k, "size") || k == "lovedMaxPerArtist" {
			if f, _ := p["default"].(float64); f != 0 {
				t.Errorf("%s manifest default %v, spec says 0", k, p["default"])
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
			_ = c.SizeFor("loved")
			_ = c.SubjectName("artistradio", "x", "d")
		}()
	}
}
