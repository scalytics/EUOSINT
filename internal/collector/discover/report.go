// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"fmt"
	"io"
)

// DiscoveryReport is the JSON output structure for a discovery run.
type DiscoveryReport struct {
	NewCandidates     []DiscoveredSource `json:"new_candidates"`
	NewCandidateCount int                `json:"new_candidate_count"`
	ExistingFeedURLs  int                `json:"existing_feed_urls"`
}

// WriteReport writes the discovery results to the output path and logs
// a summary to the writer.
func WriteReport(outputPath string, discovered []DiscoveredSource, existingCount int, stdout io.Writer) error {
	report := DiscoveryReport{
		NewCandidates:     discovered,
		NewCandidateCount: len(discovered),
		ExistingFeedURLs:  existingCount,
	}
	if report.NewCandidates == nil {
		report.NewCandidates = []DiscoveredSource{}
	}
	if err := writeJSON(outputPath, report); err != nil {
		return fmt.Errorf("write discovery report: %w", err)
	}
	fmt.Fprintf(stdout, "Discovery complete: %d new candidates found (%d existing feed URLs in registry) -> %s\n",
		len(discovered), existingCount, outputPath)
	return nil
}
