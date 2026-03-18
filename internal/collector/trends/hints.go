// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package trends

import (
	"fmt"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
)

// categorySearchTemplates maps alert categories to the kind of source the
// discovery system should look for when a trend spikes in that category.
var categorySearchTemplates = map[string]struct {
	authorityTypes []string
	searchSuffix   string
}{
	"cyber_advisory": {
		authorityTypes: []string{"cert", "national_security"},
		searchSuffix:   "CERT cybersecurity advisory RSS feed",
	},
	"terrorism": {
		authorityTypes: []string{"national_security", "intelligence", "police"},
		searchSuffix:   "counter-terrorism security agency official feed",
	},
	"conflict_monitoring": {
		authorityTypes: []string{"national_security", "military", "government"},
		searchSuffix:   "government security press office RSS feed",
	},
	"public_safety": {
		authorityTypes: []string{"public_safety_program", "government"},
		searchSuffix:   "government civil protection emergency feed",
	},
	"environmental_disaster": {
		authorityTypes: []string{"regulatory", "public_safety_program"},
		searchSuffix:   "environmental disaster agency official feed",
	},
	"public_appeal": {
		authorityTypes: []string{"police", "government"},
		searchSuffix:   "police public appeal official RSS feed",
	},
	"maritime_security": {
		authorityTypes: []string{"coast_guard", "navy", "maritime"},
		searchSuffix:   "maritime security coast guard official feed",
	},
	"fraud": {
		authorityTypes: []string{"police", "regulatory", "government"},
		searchSuffix:   "fraud consumer protection official press releases",
	},
}

// HintsToCandidates converts discovery hints into source candidates that
// the existing discovery pipeline can probe and vet.
func HintsToCandidates(hints []DiscoveryHint) []model.SourceCandidate {
	var candidates []model.SourceCandidate

	for _, hint := range hints {
		template, ok := categorySearchTemplates[hint.Category]
		if !ok {
			// Fall back to generic government press search.
			template = struct {
				authorityTypes []string
				searchSuffix   string
			}{
				authorityTypes: []string{"government"},
				searchSuffix:   "government official press releases RSS feed",
			}
		}

		// Build a search-friendly authority name from the trending terms.
		topTerms := hint.Terms
		if len(topTerms) > 5 {
			topTerms = topTerms[:5]
		}
		searchQuery := strings.Join(topTerms, " ") + " " + template.searchSuffix

		authorityType := "government"
		if len(template.authorityTypes) > 0 {
			authorityType = template.authorityTypes[0]
		}

		// Extract region info for the candidate.
		region := hint.Region
		if region == "" {
			region = "INT"
		}

		candidates = append(candidates, model.SourceCandidate{
			AuthorityName: searchQuery,
			AuthorityType: authorityType,
			Category:      hint.Category,
			Region:        region,
			Notes:         fmt.Sprintf("trend-hint: %s", hint.Reason),
		})
	}

	return candidates
}

// MergeCandidateQueue appends trend-generated candidates to an existing
// candidate list, deduplicating by notes prefix to avoid re-queuing the
// same trend hint across cycles.
func MergeCandidateQueue(existing []model.SourceCandidate, trendCandidates []model.SourceCandidate) []model.SourceCandidate {
	seen := map[string]bool{}
	for _, c := range existing {
		if strings.HasPrefix(c.Notes, "trend-hint:") {
			seen[c.Notes] = true
		}
	}

	merged := make([]model.SourceCandidate, len(existing))
	copy(merged, existing)
	for _, c := range trendCandidates {
		if seen[c.Notes] {
			continue
		}
		seen[c.Notes] = true
		merged = append(merged, c)
	}
	return merged
}
