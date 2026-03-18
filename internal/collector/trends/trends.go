// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

// Package trends detects term-frequency spikes across collection cycles and
// emits discovery hints so the discovery system can find new sources for
// emerging topics.
package trends

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/scalytics/euosint/internal/collector/model"
)

// Spike represents a significant increase in term frequency compared to
// the rolling baseline — signals that a topic is trending.
type Spike struct {
	Term        string  `json:"term"`
	Category    string  `json:"category"`
	Region      string  `json:"region"`
	TodayCount  int     `json:"today_count"`
	AvgCount    float64 `json:"avg_count"`
	Ratio       float64 `json:"ratio"` // today / avg
	SampleTitle string  `json:"sample_title,omitempty"`
}

// DiscoveryHint is a structured signal for the discovery system to find
// new sources covering a trending topic.
type DiscoveryHint struct {
	Terms    []string `json:"terms"`
	Category string   `json:"category"`
	Region   string   `json:"region"`
	Reason   string   `json:"reason"`
}

// Detector analyses alert term frequencies and detects spikes.
type Detector struct {
	db *sql.DB
}

// New creates a trend detector backed by the given SQLite database.
// Callers should call Init() to create the schema.
func New(db *sql.DB) *Detector {
	return &Detector{db: db}
}

// Init creates the term_trends table if it doesn't exist.
func (d *Detector) Init(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS term_trends (
  term       TEXT NOT NULL,
  bucket     TEXT NOT NULL,
  category   TEXT NOT NULL,
  region     TEXT NOT NULL,
  count      INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (term, bucket, category, region)
)`)
	if err != nil {
		return fmt.Errorf("create term_trends table: %w", err)
	}
	_, err = d.db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_term_trends_bucket ON term_trends(bucket)`)
	return err
}

// Record extracts significant terms from alerts and stores their counts
// for the given day bucket.
func (d *Detector) Record(ctx context.Context, alerts []model.Alert, now time.Time) error {
	bucket := now.UTC().Format("2006-01-02")

	// Aggregate term counts from alert titles.
	type key struct{ term, category, region string }
	counts := map[key]int{}
	for _, a := range alerts {
		if a.Severity == "info" || a.Category == "informational" {
			continue // skip noise
		}
		terms := extractSignificantTerms(a.Title)
		for _, t := range terms {
			k := key{term: t, category: a.Category, region: a.RegionTag}
			counts[k]++
		}
	}

	if len(counts) == 0 {
		return nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin trend tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO term_trends (term, bucket, category, region, count)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (term, bucket, category, region)
DO UPDATE SET count = count + excluded.count`)
	if err != nil {
		return fmt.Errorf("prepare trend insert: %w", err)
	}
	defer stmt.Close()

	for k, count := range counts {
		if _, err := stmt.ExecContext(ctx, k.term, bucket, k.category, k.region, count); err != nil {
			return fmt.Errorf("insert trend %q: %w", k.term, err)
		}
	}
	return tx.Commit()
}

// DetectSpikes finds terms whose current-day count significantly exceeds
// their rolling average over the lookback window.
func (d *Detector) DetectSpikes(ctx context.Context, now time.Time, lookbackDays int, minRatio float64, minCount int) ([]Spike, error) {
	if lookbackDays < 2 {
		lookbackDays = 7
	}
	if minRatio <= 0 {
		minRatio = 3.0
	}
	if minCount < 1 {
		minCount = 3
	}

	today := now.UTC().Format("2006-01-02")
	windowStart := now.UTC().AddDate(0, 0, -lookbackDays).Format("2006-01-02")

	rows, err := d.db.QueryContext(ctx, `
SELECT term, category, region,
  COALESCE(SUM(CASE WHEN bucket = ? THEN count END), 0) AS today_count,
  COALESCE(AVG(CASE WHEN bucket < ? AND bucket >= ? THEN count END), 0) AS avg_count
FROM term_trends
WHERE bucket >= ?
GROUP BY term, category, region
HAVING today_count >= ? AND (avg_count = 0 OR today_count > avg_count * ?)`,
		today, today, windowStart, windowStart, minCount, minRatio)
	if err != nil {
		return nil, fmt.Errorf("detect spikes: %w", err)
	}
	defer rows.Close()

	var spikes []Spike
	for rows.Next() {
		var s Spike
		if err := rows.Scan(&s.Term, &s.Category, &s.Region, &s.TodayCount, &s.AvgCount); err != nil {
			return nil, fmt.Errorf("scan spike: %w", err)
		}
		if s.AvgCount > 0 {
			s.Ratio = float64(s.TodayCount) / s.AvgCount
		} else {
			s.Ratio = float64(s.TodayCount) // new term, treat as infinite ratio
		}
		spikes = append(spikes, s)
	}
	return spikes, rows.Err()
}

// BuildHints groups spikes into discovery hints by category+region.
func BuildHints(spikes []Spike) []DiscoveryHint {
	type hintKey struct{ category, region string }
	grouped := map[hintKey][]Spike{}
	for _, s := range spikes {
		k := hintKey{category: s.Category, region: s.Region}
		grouped[k] = append(grouped[k], s)
	}

	var hints []DiscoveryHint
	for k, group := range grouped {
		terms := make([]string, 0, len(group))
		var topSpike Spike
		for _, s := range group {
			terms = append(terms, s.Term)
			if s.Ratio > topSpike.Ratio {
				topSpike = s
			}
		}
		reason := fmt.Sprintf("%s mentions %.0fx above %d-day avg in %s",
			topSpike.Term, topSpike.Ratio, 7, k.region)
		if k.region == "" {
			reason = fmt.Sprintf("%s mentions %.0fx above %d-day avg",
				topSpike.Term, topSpike.Ratio, 7)
		}
		hints = append(hints, DiscoveryHint{
			Terms:    terms,
			Category: k.category,
			Region:   k.region,
			Reason:   reason,
		})
	}
	return hints
}

// Prune removes trend data older than the retention period.
func (d *Detector) Prune(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays < 1 {
		retentionDays = 30
	}
	cutoff := now.UTC().AddDate(0, 0, -retentionDays).Format("2006-01-02")
	_, err := d.db.ExecContext(ctx, `DELETE FROM term_trends WHERE bucket < ?`, cutoff)
	return err
}

// ── Term extraction ─────────────────────────────────────────────────────

var (
	wordRe = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'-]*`)

	// trendStopwords are terms too common or generic to be meaningful trend
	// signals. Kept separate from geocoder stopwords — different purpose.
	trendStopwords = map[string]bool{
		// English function words
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"has": true, "its": true, "his": true, "how": true, "man": true,
		"new": true, "now": true, "old": true, "see": true, "way": true,
		"who": true, "did": true, "get": true, "let": true, "say": true,
		"she": true, "too": true, "use": true, "with": true, "from": true,
		"that": true, "this": true, "will": true, "been": true, "have": true,
		"more": true, "when": true, "some": true, "they": true, "than": true,
		"them": true, "then": true, "what": true, "were": true, "also": true,
		"into": true, "over": true, "such": true, "just": true, "only": true,
		"very": true, "after": true, "about": true, "which": true, "their": true,
		"there": true, "other": true, "being": true, "could": true, "would": true,
		"should": true, "through": true, "between": true,
		// OSINT institutional noise (appears in nearly every feed)
		"update": true, "report": true, "press": true, "release": true,
		"notice": true, "bulletin": true, "statement": true, "news": true,
		"latest": true, "official": true, "source": true, "information": true,
		"published": true, "issued": true, "date": true, "today": true,
		"national": true, "federal": true, "public": true, "general": true,
		"authority": true, "department": true, "ministry": true, "office": true,
		"agency": true, "service": true, "government": true, "commission": true,
	}
)

// extractSignificantTerms pulls meaningful terms from an alert title.
// Returns lowercased terms that pass length and stopword filters.
func extractSignificantTerms(title string) []string {
	matches := wordRe.FindAllString(title, -1)
	seen := map[string]bool{}
	var terms []string
	for _, m := range matches {
		lower := strings.ToLower(m)
		if len(lower) < 3 || len(lower) > 40 {
			continue
		}
		if trendStopwords[lower] {
			continue
		}
		if isNumericOnly(lower) {
			continue
		}
		if seen[lower] {
			continue
		}
		seen[lower] = true
		terms = append(terms, lower)
	}
	return terms
}

func isNumericOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// ── Sample title lookup ─────────────────────────────────────────────────

// AnnotateSpikesWithSamples fills in SampleTitle for each spike by finding
// a matching alert title.
func AnnotateSpikesWithSamples(spikes []Spike, alerts []model.Alert) {
	termIndex := map[string]string{} // term → first matching title
	for _, a := range alerts {
		lower := strings.ToLower(a.Title)
		terms := extractSignificantTerms(a.Title)
		for _, t := range terms {
			if _, ok := termIndex[t]; !ok {
				if strings.Contains(lower, t) {
					termIndex[t] = a.Title
				}
			}
		}
	}
	for i := range spikes {
		if title, ok := termIndex[spikes[i].Term]; ok {
			spikes[i].SampleTitle = title
		}
	}
}

// SpikeScore returns a 0-1 score for how significant a spike is,
// combining ratio strength and absolute count.
func SpikeScore(s Spike) float64 {
	// Log-scaled ratio component (ratio of 3 → ~0.5, ratio of 10 → ~0.8)
	ratioScore := math.Min(1.0, math.Log2(s.Ratio)/math.Log2(20))
	// Log-scaled count component (3 → ~0.3, 10 → ~0.6, 50 → ~0.9)
	countScore := math.Min(1.0, math.Log2(float64(s.TodayCount))/math.Log2(100))
	return 0.6*ratioScore + 0.4*countScore
}
