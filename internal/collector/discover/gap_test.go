// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"bytes"
	"testing"

	"github.com/scalytics/kafSIEM/internal/collector/model"
)

func TestAnalyzeGaps_FindsMissing(t *testing.T) {
	// A registry covering all expanded categories for Germany — everything else is a gap.
	var sources []model.RegistrySource
	for _, cat := range expandedCategories {
		sources = append(sources, model.RegistrySource{
			Category: cat,
			Source:   model.SourceMetadata{CountryCode: "DE"},
		})
	}

	var buf bytes.Buffer
	gaps := AnalyzeGaps(sources, &buf)

	if len(gaps) == 0 {
		t.Fatal("expected gap candidates, got none")
	}

	// Norway should be missing all categories.
	norwayCats := map[string]bool{}
	for _, g := range gaps {
		if g.CountryCode == "NO" {
			norwayCats[g.Category] = true
		}
	}
	for _, cat := range expandedCategories {
		if !norwayCats[cat] {
			t.Errorf("expected Norway gap for %s", cat)
		}
	}

	// Germany should NOT appear (fully covered).
	for _, g := range gaps {
		if g.CountryCode == "DE" {
			t.Errorf("unexpected gap for Germany: %s", g.Category)
		}
	}
}

func TestAnalyzeGaps_FullCoverage(t *testing.T) {
	// Build a registry that covers every target country+category.
	var sources []model.RegistrySource
	for _, target := range targetCountries {
		for _, cat := range target.Categories {
			sources = append(sources, model.RegistrySource{
				Category: cat,
				Source:   model.SourceMetadata{CountryCode: target.CountryCode},
			})
		}
	}

	var buf bytes.Buffer
	gaps := AnalyzeGaps(sources, &buf)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps with full coverage, got %d", len(gaps))
	}
}
