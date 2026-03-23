package normalize

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ---------- incident_terms.json loading ----------

func TestIncidentTermsLoad(t *testing.T) {
	idx := loadIncidentTerms()
	if idx == nil {
		t.Fatal("loadIncidentTerms returned nil")
	}
	for _, tier := range []string{"critical", "high", "medium", "incident_indicators"} {
		if _, ok := idx.tiersPerLang[tier]; !ok {
			t.Errorf("missing tier %q", tier)
		}
	}
}

func TestIncidentTermsHasIcelandic(t *testing.T) {
	idx := loadIncidentTerms()
	if idx == nil {
		t.Fatal("loadIncidentTerms returned nil")
	}
	// "morð" (murder) should be in the critical tier for Icelandic
	terms := idx.tiersPerLang["critical"]["is"]
	found := false
	for _, w := range terms {
		if w == "morð" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Icelandic 'morð' (murder) not found in critical tier")
	}
}

func TestIncidentTermsMinLength(t *testing.T) {
	idx := loadIncidentTerms()
	if idx == nil {
		t.Fatal("loadIncidentTerms returned nil")
	}
	for tier, langs := range idx.tiersPerLang {
		for lang, terms := range langs {
			for _, term := range terms {
				runeLen := len([]rune(term))
				// CJK terms are valid at 2 chars; Latin terms need 3+
				isCJK := false
				for _, r := range term {
					if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3040 && r <= 0x309F || r >= 0x30A0 && r <= 0x30FF || r >= 0xAC00 && r <= 0xD7AF {
						isCJK = true
						break
					}
				}
				minLen := 3
				if isCJK {
					minLen = 2
				}
				if runeLen < minLen {
					t.Errorf("tier=%s lang=%s: term %q is too short (%d runes, min %d)", tier, lang, term, runeLen, minLen)
				}
			}
		}
	}
}

// ---------- language-scoped matching ----------

func TestMatchesIncidentTermIcelandic(t *testing.T) {
	cases := []struct {
		text string
		lang string
		want string
	}{
		// Icelandic dead/accident — should match incident_indicators
		{"Lést í vinnuslysi", "is", "incident_indicators"},
		// Icelandic assault — should match high
		{"Alvarleg líkamsárás – fimm í gæsluvarðhaldi", "is", "high"},
		// Icelandic murder — should match critical
		{"Morð á Akureyri", "is", "critical"},
		// Icelandic earthquake
		{"Jarðskjálfti á Suðurlandi", "is", "critical"},
	}
	for _, tc := range cases {
		got := matchesIncidentTerm(tc.text, tc.lang)
		if got != tc.want {
			t.Errorf("matchesIncidentTerm(%q, %q) = %q, want %q", tc.text, tc.lang, got, tc.want)
		}
	}
}

func TestMatchesIncidentTermMultipleLanguages(t *testing.T) {
	cases := []struct {
		text string
		lang string
		want string
	}{
		// German fire
		{"Großbrand in Hamburger Lagerhalle", "de", "high"},
		// French murder
		{"Meurtre dans le 18e arrondissement", "fr", "critical"},
		// Spanish earthquake
		{"Terremoto de magnitud 6.2 sacude el sur de México", "es", "critical"},
		// Hungarian assault
		{"Támadás a belvárosban", "hu", "high"},
		// Finnish shooting
		{"Ampuminen Helsingissä", "fi", "high"},
		// Turkish earthquake
		{"İstanbul'da deprem meydana geldi", "tr", "critical"},
		// Polish flood
		{"Powódź na Dolnym Śląsku", "pl", "high"},
		// Japanese earthquake
		{"東京で地震が発生", "ja", "critical"},
		// Arabic explosion
		{"انفجار في وسط المدينة", "ar", "critical"},
		// Swedish arrested
		{"Gripen för grovt narkotikabrott", "sv", "medium"},
		// Portuguese robbery
		{"Assalto a banco no centro de Lisboa", "pt", "high"},
	}
	for _, tc := range cases {
		got := matchesIncidentTerm(tc.text, tc.lang)
		if got != tc.want {
			t.Errorf("matchesIncidentTerm(%q, %q) = %q, want %q", tc.text, tc.lang, got, tc.want)
		}
	}
}

func TestMatchesIncidentTermSkipsEnglish(t *testing.T) {
	// English should always return "" — handled by keyword lists
	got := matchesIncidentTerm("Murder in downtown area", "en")
	if got != "" {
		t.Errorf("expected empty for English, got %q", got)
	}
	got = matchesIncidentTerm("Explosion at factory", "")
	if got != "" {
		t.Errorf("expected empty for empty lang, got %q", got)
	}
}

func TestMatchesIncidentTermNoFalsePositives(t *testing.T) {
	// Icelandic terms must NOT match against non-Icelandic text
	got := matchesIncidentTerm("This is a normal sentence about nothing", "is")
	if got != "" {
		t.Errorf("false positive: Icelandic matching returned %q for English text", got)
	}

	// German "Brand" should NOT match when lang is "fr"
	got = matchesIncidentTerm("Die Marke Brand ist sehr beliebt", "fr")
	if got != "" {
		t.Errorf("false positive: French matching returned %q for German text", got)
	}

	// "morð" should NOT match when lang is "de" (wrong language)
	got = matchesIncidentTerm("Ein morð passierte", "de")
	if got != "" {
		t.Errorf("false positive: German matching returned %q for Icelandic word", got)
	}
}

// ---------- hasIncidentIndicators ----------

func TestHasIncidentIndicatorsEnglish(t *testing.T) {
	cases := []struct {
		title string
		lang  string
		want  bool
	}{
		{"Fatal crash on highway", "en", true},
		{"Explosion at chemical plant", "en", true},
		{"Three arrested in drug raid", "en", true},
		{"Press conference held", "en", false},
		{"Workshop on cybersecurity", "en", false},
	}
	for _, tc := range cases {
		got := hasIncidentIndicators(tc.title, tc.lang)
		if got != tc.want {
			t.Errorf("hasIncidentIndicators(%q, %q) = %v, want %v", tc.title, tc.lang, got, tc.want)
		}
	}
}

func TestHasIncidentIndicatorsIcelandic(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		// "Died in workplace accident"
		{"Lést í vinnuslysi", true},
		// "Workplace accident in Reykjavík"
		{"Vinnuslys í Korngörðum í Reykjavík", true},
		// "Serious assault – five in custody"
		{"Alvarleg líkamsárás – fimm í gæsluvarðhaldi", true},
		// "About firearms" (no incident)
		{"Um skotvopn og vörslur þeirra", false},
		// "Suspicious emails" (no incident)
		{"Grunsamlegir tölvupóstar", false},
	}
	for _, tc := range cases {
		got := hasIncidentIndicators(tc.title, "is")
		if got != tc.want {
			t.Errorf("hasIncidentIndicators(%q, 'is') = %v, want %v", tc.title, got, tc.want)
		}
	}
}

func TestHasIncidentIndicatorsHungarian(t *testing.T) {
	// "Murder in Budapest"
	if !hasIncidentIndicators("Gyilkosság Budapesten", "hu") {
		t.Error("expected true for Hungarian 'gyilkosság'")
	}
	// "Flood in Szeged"
	if !hasIncidentIndicators("Árvíz Szegeden", "hu") {
		t.Error("expected true for Hungarian 'árvíz'")
	}
}

// ---------- inferSeverity with multilingual fallback ----------

func TestInferSeverityMultilingual(t *testing.T) {
	cases := []struct {
		title    string
		lang     string
		fallback string
		want     string
	}{
		// Icelandic murder → critical
		{"Morð á Akureyri", "is", "info", "critical"},
		// German fire → high
		{"Brand in Lagerhalle", "de", "info", "high"},
		// Swedish "bankrån" contains "rån" (robbery) → high
		{"Gripen efter bankrån", "sv", "info", "high"},
		// Icelandic non-incident → falls back to fallback
		{"Grunsamlegir tölvupóstar", "is", "info", "info"},
		// English still uses keyword path, not multilingual
		{"Explosion at factory", "en", "info", "critical"},
	}
	for _, tc := range cases {
		got := inferSeverity(tc.title, tc.fallback, tc.lang)
		if got != tc.want {
			t.Errorf("inferSeverity(%q, %q, %q) = %q, want %q", tc.title, tc.fallback, tc.lang, got, tc.want)
		}
	}
}

// ---------- countryToLang coverage ----------

func TestCountryToLangCoversRegistry(t *testing.T) {
	paths := []string{
		"../../../registry/source_registry.json",
		"/app/registry/source_registry.json",
	}

	type regEntry struct {
		Source struct {
			CountryCode string `json:"country_code"`
		} `json:"source"`
	}

	var entries []regEntry
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(body, &entries); err != nil {
			continue
		}
		break
	}
	if len(entries) == 0 {
		t.Skip("source_registry.json not found")
	}

	unmapped := make(map[string]bool)
	for _, e := range entries {
		cc := strings.ToUpper(e.Source.CountryCode)
		if cc == "" {
			continue
		}
		if _, ok := countryToLang[cc]; !ok {
			unmapped[cc] = true
		}
	}
	if len(unmapped) > 0 {
		codes := make([]string, 0, len(unmapped))
		for cc := range unmapped {
			codes = append(codes, cc)
		}
		t.Errorf("registry country codes missing from countryToLang: %v", codes)
	}
}

func TestCountryToLangNoEmptyValues(t *testing.T) {
	for cc, lang := range countryToLang {
		if lang == "" {
			t.Errorf("countryToLang[%q] = empty string", cc)
		}
		if len(lang) < 2 {
			t.Errorf("countryToLang[%q] = %q, expected 2+ char ISO-639-1 code", cc, lang)
		}
	}
}

func TestCountryToLangMajorCountries(t *testing.T) {
	// Spot check major countries
	expect := map[string]string{
		"US": "en", "DE": "de", "FR": "fr", "JP": "ja", "BR": "pt",
		"RU": "ru", "CN": "zh", "IS": "is", "HU": "hu", "TR": "tr",
		"SA": "ar", "IL": "he", "KR": "ko", "TH": "th", "PL": "pl",
		"EE": "et", "LV": "lv", "LT": "lt", "GE": "ka", "AM": "hy",
	}
	for cc, want := range expect {
		got, ok := countryToLang[cc]
		if !ok {
			t.Errorf("countryToLang missing %q", cc)
		} else if got != want {
			t.Errorf("countryToLang[%q] = %q, want %q", cc, got, want)
		}
	}
}
