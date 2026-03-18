package normalize

import (
	"math"
	"testing"
)

func TestExtractCoordinates_DMS(t *testing.T) {
	text := "Vessel attacked at 35°54'N 14°31'E near Malta"
	lat, lng, ok := ExtractCoordinates(text)
	if !ok {
		t.Fatal("expected coordinates to be found")
	}
	if math.Abs(lat-35.9) > 0.01 || math.Abs(lng-14.517) > 0.01 {
		t.Errorf("got lat=%f lng=%f, want ~35.9, ~14.517", lat, lng)
	}
}

func TestExtractCoordinates_DMSWithSeconds(t *testing.T) {
	text := `Position: 12°03'30"N 045°02'15"E`
	lat, lng, ok := ExtractCoordinates(text)
	if !ok {
		t.Fatal("expected coordinates to be found")
	}
	expectedLat := 12.0 + 3.0/60 + 30.0/3600
	expectedLng := 45.0 + 2.0/60 + 15.0/3600
	if math.Abs(lat-expectedLat) > 0.001 || math.Abs(lng-expectedLng) > 0.001 {
		t.Errorf("got lat=%f lng=%f, want ~%f ~%f", lat, lng, expectedLat, expectedLng)
	}
}

func TestExtractCoordinates_DecimalMinutes(t *testing.T) {
	text := "Incident at 12°03.5'N 045°02.1'E in the Gulf of Aden"
	lat, lng, ok := ExtractCoordinates(text)
	if !ok {
		t.Fatal("expected coordinates to be found")
	}
	expectedLat := 12.0 + 3.5/60
	expectedLng := 45.0 + 2.1/60
	if math.Abs(lat-expectedLat) > 0.001 || math.Abs(lng-expectedLng) > 0.001 {
		t.Errorf("got lat=%f lng=%f, want ~%f ~%f", lat, lng, expectedLat, expectedLng)
	}
}

func TestExtractCoordinates_Decimal(t *testing.T) {
	tests := []struct {
		name string
		text string
		lat  float64
		lng  float64
	}{
		{"suffix form", "Position 35.9N 14.5E reported", 35.9, 14.5},
		{"prefix form", "At N35.9 E14.5 a ship was attacked", 35.9, 14.5},
		{"south west", "Wreck found at 34.5S 58.3W off Argentina", -34.5, -58.3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lng, ok := ExtractCoordinates(tt.text)
			if !ok {
				t.Fatal("expected coordinates to be found")
			}
			if math.Abs(lat-tt.lat) > 0.01 || math.Abs(lng-tt.lng) > 0.01 {
				t.Errorf("got lat=%f lng=%f, want %f %f", lat, lng, tt.lat, tt.lng)
			}
		})
	}
}

func TestExtractCoordinates_BareDecimal(t *testing.T) {
	tests := []struct {
		name string
		text string
		lat  float64
		lng  float64
	}{
		{"LLM output", "31.5050, 34.4667", 31.5050, 34.4667},
		{"negative lng", "-33.8688, 151.2093", -33.8688, 151.2093},
		{"with context", "Location: 48.8566, 2.3522 (Paris)", 48.8566, 2.3522},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lng, ok := ExtractCoordinates(tt.text)
			if !ok {
				t.Fatal("expected coordinates to be found")
			}
			if math.Abs(lat-tt.lat) > 0.001 || math.Abs(lng-tt.lng) > 0.001 {
				t.Errorf("got lat=%f lng=%f, want %f %f", lat, lng, tt.lat, tt.lng)
			}
		})
	}
}

func TestExtractCoordinates_NoMatch(t *testing.T) {
	texts := []string{
		"Ship attacked near Strait of Hormuz",
		"No coordinates in this report",
		"Temperature was 35 degrees",
	}
	for _, text := range texts {
		if _, _, ok := ExtractCoordinates(text); ok {
			t.Errorf("expected no match for %q", text)
		}
	}
}

func TestMatchMaritimeRegion(t *testing.T) {
	tests := []struct {
		text     string
		wantName string
		wantLat  float64
		wantLng  float64
		wantOK   bool
	}{
		{"Attack in the Gulf of Aden", "Gulf of Aden", 12.0, 45.0, true},
		{"Tanker seized near Strait of Hormuz", "Strait of Hormuz", 26.5, 56.3, true},
		{"Piracy alert in the Malacca Strait area", "Malacca Strait", 2.5, 101.0, true},
		{"Ship transiting Suez Canal", "Suez Canal", 30.5, 32.3, true},
		{"Russian vessel in Mediterranean Sea near Malta", "Mediterranean Sea", 35.0, 18.0, true},
		{"Nothing maritime here", "", 0, 0, false},
		// Longest match wins: "Strait of Malacca" over "Malacca"
		{"Incident in the Strait of Malacca", "Strait of Malacca", 2.5, 101.0, true},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			lat, lng, name, ok := MatchMaritimeRegion(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if name != tt.wantName {
				t.Errorf("name=%q, want %q", name, tt.wantName)
			}
			if math.Abs(lat-tt.wantLat) > 0.1 || math.Abs(lng-tt.wantLng) > 0.1 {
				t.Errorf("got lat=%f lng=%f, want %f %f", lat, lng, tt.wantLat, tt.wantLng)
			}
		})
	}
}
