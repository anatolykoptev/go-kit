package skills

import "testing"

func TestParseMetadata_JSON_UnknownTopLevelKeys_LandInExtra(t *testing.T) {
	m := ParseMetadata(jsonFM(`{"name":"foo","model":"fast","retries":3,"strict":true,"nested":{"a":1},"list":["x","y"]}`))

	for key, want := range map[string]string{
		"model":   "fast",
		"retries": "3",
		"strict":  "true",
		"nested":  `{"a":1}`,
		"list":    `["x","y"]`,
	} {
		if got, ok := m.Extra[key]; !ok || got != want {
			t.Errorf("Extra[%q] = (%q, %v), want (%q, true)", key, got, ok, want)
		}
	}
}

// YAML and JSON frontmatter carrying the same fields must agree on Extra.
func TestParseMetadata_JSON_UnknownKeys_MatchYAML(t *testing.T) {
	y := ParseMetadata(yamlFM("name: foo\nmodel: fast\nowner: team-a"))
	j := ParseMetadata(jsonFM(`{"name":"foo","model":"fast","owner":"team-a"}`))

	if len(y.Extra) != len(j.Extra) {
		t.Fatalf("Extra differs: yaml=%v json=%v", y.Extra, j.Extra)
	}
	for k, v := range y.Extra {
		if j.Extra[k] != v {
			t.Errorf("Extra[%q]: yaml=%q json=%q", k, v, j.Extra[k])
		}
	}
}

func TestParseMetadata_JSON_KnownKeysAndMetadataMap_NotDuplicatedIntoExtra(t *testing.T) {
	m := ParseMetadata(jsonFM(`{"name":"foo","description":"d","license":"MIT","compatibility":"c","when_to_use":"w","allowed-tools":["A"],"disable-model-invocation":true,"user-invocable":false,"version":"1","locale":"en","tags":["t"],"NAME":"ignored-case-variant","metadata":{"author":"bob"}}`))

	if len(m.Extra) != 1 || m.Extra["author"] != "bob" {
		t.Errorf("Extra = %v, want only author=bob (known keys, even case variants, are not extension fields)", m.Extra)
	}
}

// The nested `metadata:` map is the designated extension map and wins over a
// top-level key of the same name.
func TestParseMetadata_JSON_MetadataMapWinsOverTopLevelDuplicate(t *testing.T) {
	m := ParseMetadata(jsonFM(`{"name":"foo","owner":"top","metadata":{"owner":"nested","author":"bob"}}`))

	if m.Extra["owner"] != "nested" {
		t.Errorf("Extra[owner] = %q, want nested", m.Extra["owner"])
	}
	if m.Extra["author"] != "bob" {
		t.Errorf("Extra[author] = %q, want bob", m.Extra["author"])
	}
}

func TestParseMetadata_JSON_NoUnknownKeys_ExtraStaysNil(t *testing.T) {
	if m := ParseMetadata(jsonFM(`{"name":"minimal"}`)); m.Extra != nil {
		t.Errorf("Extra = %v, want nil", m.Extra)
	}
}
