// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"encoding/json"
	"testing"
)

func TestParseFeodo(t *testing.T) {
	entries := []FeodoEntry{
		{
			IPAddress:  "192.168.1.1",
			Port:       443,
			Status:     "online",
			ASNumber:   12345,
			ASName:     "Evil ISP",
			Country:    "RU",
			FirstSeen:  "2026-03-10",
			LastOnline: "2026-03-17",
			Malware:    "Dridex",
		},
		{
			IPAddress:  "10.0.0.1",
			Port:       8080,
			Status:     "offline",
			ASNumber:   67890,
			ASName:     "Good ISP",
			Country:    "US",
			FirstSeen:  "2026-01-01",
			LastOnline: "2026-02-15",
			Malware:    "TrickBot",
		},
		{
			IPAddress:  "172.16.0.1",
			Port:       447,
			Status:     "online",
			ASNumber:   11111,
			ASName:     "Another ISP",
			Country:    "DE",
			FirstSeen:  "2026-03-15",
			LastOnline: "2026-03-17",
			Malware:    "QakBot",
		},
	}
	body, _ := json.Marshal(entries)

	items, err := ParseFeodo(body)
	if err != nil {
		t.Fatal(err)
	}
	// Only online entries should be returned.
	if len(items) != 2 {
		t.Fatalf("expected 2 online items, got %d", len(items))
	}

	dridex := items[0]
	if dridex.IPAddress != "192.168.1.1" {
		t.Errorf("expected IP=192.168.1.1, got %q", dridex.IPAddress)
	}
	if dridex.Malware != "Dridex" {
		t.Errorf("expected Malware=Dridex, got %q", dridex.Malware)
	}
	if dridex.Country != "RU" {
		t.Errorf("expected Country=RU, got %q", dridex.Country)
	}
	if dridex.Published != "2026-03-17" {
		t.Errorf("expected Published=2026-03-17, got %q", dridex.Published)
	}

	qakbot := items[1]
	if qakbot.Malware != "QakBot" {
		t.Errorf("expected Malware=QakBot, got %q", qakbot.Malware)
	}
}

func TestParseFeodoEmpty(t *testing.T) {
	items, err := ParseFeodo([]byte("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}
