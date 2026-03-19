// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import "testing"

func TestGeocodeText(t *testing.T) {
	tests := []struct {
		text     string
		wantCode string
		wantOK   bool
	}{
		{"Crisis in Myanmar's Rakhine State", "MM", true},
		{"Ukraine Conflict Monitor Update", "UA", true},
		{"Ethiopia's Tigray: A Fragile Peace", "ET", true},
		{"South Sudan Violence Escalates", "SS", true},
		{"Sudanese Military Conflict Deepens", "SD", true},
		{"Israeli Strikes on Gaza Intensify", "PS", true}, // Gaza → PS
		{"DRC Eastern Congo Humanitarian Emergency", "CD", true},
		{"Sahel Region Security Briefing", "ML", true},
		{"During last nights reported airstrike in Syria Russian experts were present and injured", "SY", true},
		{"Justice for Palestinian Women Demands End to Occupation, Reparations, Accountability, Experts Tell Rights Committee", "PS", true},
		{"Weekly Global Summary Report", "", false},
		{"New Policy Framework Released", "", false},
	}
	for _, tt := range tests {
		_, _, code, ok := geocodeText(tt.text)
		if ok != tt.wantOK {
			t.Errorf("geocodeText(%q): ok=%v, want %v", tt.text, ok, tt.wantOK)
			continue
		}
		if code != tt.wantCode {
			t.Errorf("geocodeText(%q): code=%q, want %q", tt.text, code, tt.wantCode)
		}
	}
}
