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
  term         TEXT NOT NULL,
  bucket       TEXT NOT NULL,
  category     TEXT NOT NULL,
  region       TEXT NOT NULL,
  count        INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (term, bucket, category, region)
)`)
	if err != nil {
		return fmt.Errorf("create term_trends table: %w", err)
	}
	// Country-level term tracking for the country digest feature.
	_, err = d.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS term_country_trends (
  term         TEXT NOT NULL,
  bucket       TEXT NOT NULL,
  country_code TEXT NOT NULL,
  category     TEXT NOT NULL,
  weight       REAL NOT NULL DEFAULT 0,
  count        INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (term, bucket, country_code, category)
)`)
	if err != nil {
		return fmt.Errorf("create term_country_trends table: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_term_trends_bucket ON term_trends(bucket)`,
		`CREATE INDEX IF NOT EXISTS idx_term_country_trends_bucket ON term_country_trends(bucket)`,
		`CREATE INDEX IF NOT EXISTS idx_term_country_trends_country ON term_country_trends(country_code, bucket)`,
	} {
		if _, err := d.db.ExecContext(ctx, idx); err != nil {
			return err
		}
	}
	return nil
}

// sourceTrustWeight returns a multiplier for how trustworthy a source type
// is as a signal. Police/CERT confirmed events weigh more than news blogs.
func sourceTrustWeight(authorityType string) float64 {
	switch strings.ToLower(authorityType) {
	case "police", "gendarmerie":
		return 2.0 // confirmed events
	case "cert", "national_security", "intelligence":
		return 1.8
	case "coast_guard", "customs", "border_guard":
		return 1.6
	case "government", "legislative", "regulatory":
		return 1.5
	case "public_safety_program":
		return 1.3
	case "diplomatic":
		return 1.2
	default:
		return 1.0
	}
}

// Record extracts significant terms from alerts and stores their counts
// for the given day bucket. Also populates country-level term tracking
// with source trust weighting.
func (d *Detector) Record(ctx context.Context, alerts []model.Alert, now time.Time) error {
	bucket := now.UTC().Format("2006-01-02")

	// Aggregate term counts from alert titles.
	type regionKey struct{ term, category, region string }
	type countryKey struct{ term, countryCode, category string }
	regionCounts := map[regionKey]int{}
	countryCounts := map[countryKey]struct {
		count  int
		weight float64
	}{}

	for _, a := range alerts {
		if a.Severity == "info" || a.Category == "informational" {
			continue // skip noise
		}
		terms := extractSignificantTerms(a.Title)
		trust := sourceTrustWeight(a.Source.AuthorityType)
		cc := strings.ToUpper(a.Source.CountryCode)

		for _, t := range terms {
			rk := regionKey{term: t, category: a.Category, region: a.RegionTag}
			regionCounts[rk]++

			if cc != "" && cc != "INT" {
				ck := countryKey{term: t, countryCode: cc, category: a.Category}
				entry := countryCounts[ck]
				entry.count++
				entry.weight += trust
				countryCounts[ck] = entry
			}
		}
	}

	if len(regionCounts) == 0 && len(countryCounts) == 0 {
		return nil
	}

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin trend tx: %w", err)
	}
	defer tx.Rollback()

	// Region-level trends (existing).
	if len(regionCounts) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO term_trends (term, bucket, category, region, count)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (term, bucket, category, region)
DO UPDATE SET count = count + excluded.count`)
		if err != nil {
			return fmt.Errorf("prepare trend insert: %w", err)
		}
		for k, count := range regionCounts {
			if _, err := stmt.ExecContext(ctx, k.term, bucket, k.category, k.region, count); err != nil {
				stmt.Close()
				return fmt.Errorf("insert trend %q: %w", k.term, err)
			}
		}
		stmt.Close()
	}

	// Country-level trends with source trust weighting.
	if len(countryCounts) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
INSERT INTO term_country_trends (term, bucket, country_code, category, weight, count)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (term, bucket, country_code, category)
DO UPDATE SET weight = weight + excluded.weight, count = count + excluded.count`)
		if err != nil {
			return fmt.Errorf("prepare country trend insert: %w", err)
		}
		for k, v := range countryCounts {
			if _, err := stmt.ExecContext(ctx, k.term, bucket, k.countryCode, k.category, v.weight, v.count); err != nil {
				stmt.Close()
				return fmt.Errorf("insert country trend %q: %w", k.term, err)
			}
		}
		stmt.Close()
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
	if _, err := d.db.ExecContext(ctx, `DELETE FROM term_trends WHERE bucket < ?`, cutoff); err != nil {
		return err
	}
	_, err := d.db.ExecContext(ctx, `DELETE FROM term_country_trends WHERE bucket < ?`, cutoff)
	return err
}

// ── Country digest ──────────────────────────────────────────────────────

// DigestTerm is a single term in a country intelligence digest.
type DigestTerm struct {
	Term        string  `json:"term"`
	Count       int     `json:"count"`
	Weight      float64 `json:"weight"`       // trust-weighted count
	AvgCount    float64 `json:"avg_count"`     // baseline daily average
	Ratio       float64 `json:"ratio"`         // today / avg (0 if new)
	Category    string  `json:"category"`      // dominant category
	SampleTitle string  `json:"sample_title,omitempty"`
	SampleURL   string  `json:"sample_url,omitempty"`
}

// CountryDigest is the top-terms summary for a single country.
type CountryDigest struct {
	CountryCode string       `json:"country_code"`
	Country     string       `json:"country"`
	Terms       []DigestTerm `json:"terms"`
	TotalAlerts int          `json:"total_alerts"`
}

// CountryDigestQuery returns the top trending terms for a specific country
// over the given window. Terms are ranked by trust-weighted count, with
// spike ratio as a tiebreaker. Returns up to `limit` terms.
func (d *Detector) CountryDigestQuery(ctx context.Context, countryCode string, now time.Time, days int, limit int) ([]DigestTerm, error) {
	if days < 1 {
		days = 7
	}
	if limit < 1 {
		limit = 10
	}

	today := now.UTC().Format("2006-01-02")
	windowStart := now.UTC().AddDate(0, 0, -days).Format("2006-01-02")

	rows, err := d.db.QueryContext(ctx, `
SELECT
  term,
  category,
  COALESCE(SUM(count), 0) AS total_count,
  COALESCE(SUM(weight), 0) AS total_weight,
  COALESCE(SUM(CASE WHEN bucket = ? THEN count END), 0) AS today_count,
  COALESCE(AVG(CASE WHEN bucket < ? AND bucket >= ? THEN count END), 0) AS avg_count
FROM term_country_trends
WHERE country_code = ? AND bucket >= ?
GROUP BY term, category
HAVING total_count >= 1
ORDER BY total_weight DESC, total_count DESC
LIMIT ?`,
		today, today, windowStart, strings.ToUpper(countryCode), windowStart, limit)
	if err != nil {
		return nil, fmt.Errorf("country digest query: %w", err)
	}
	defer rows.Close()

	var terms []DigestTerm
	for rows.Next() {
		var t DigestTerm
		var todayCount int
		if err := rows.Scan(&t.Term, &t.Category, &t.Count, &t.Weight, &todayCount, &t.AvgCount); err != nil {
			return nil, fmt.Errorf("scan digest term: %w", err)
		}
		if t.AvgCount > 0 {
			t.Ratio = math.Round(float64(todayCount)/t.AvgCount*10) / 10
		}
		t.Weight = math.Round(t.Weight*10) / 10
		terms = append(terms, t)
	}
	return terms, rows.Err()
}

// AllCountryDigests returns digests for all countries that have data in
// the given window. Used for the overview/dashboard.
func (d *Detector) AllCountryDigests(ctx context.Context, now time.Time, days int, termsPerCountry int) ([]CountryDigest, error) {
	if days < 1 {
		days = 7
	}
	if termsPerCountry < 1 {
		termsPerCountry = 10
	}
	windowStart := now.UTC().AddDate(0, 0, -days).Format("2006-01-02")

	// Get all country codes with data.
	rows, err := d.db.QueryContext(ctx, `
SELECT DISTINCT country_code
FROM term_country_trends
WHERE bucket >= ?
ORDER BY country_code`, windowStart)
	if err != nil {
		return nil, fmt.Errorf("list digest countries: %w", err)
	}
	var codes []string
	for rows.Next() {
		var cc string
		if err := rows.Scan(&cc); err != nil {
			rows.Close()
			return nil, err
		}
		codes = append(codes, cc)
	}
	rows.Close()

	var digests []CountryDigest
	for _, cc := range codes {
		terms, err := d.CountryDigestQuery(ctx, cc, now, days, termsPerCountry)
		if err != nil {
			continue
		}
		if len(terms) == 0 {
			continue
		}
		digests = append(digests, CountryDigest{
			CountryCode: cc,
			Terms:       terms,
		})
	}
	return digests, nil
}

// AnnotateDigestWithSamples fills in SampleTitle and SampleURL for each
// digest term by finding a matching alert.
func AnnotateDigestWithSamples(terms []DigestTerm, alerts []model.Alert, countryCode string) {
	for i := range terms {
		for _, a := range alerts {
			if strings.ToUpper(a.Source.CountryCode) != strings.ToUpper(countryCode) {
				continue
			}
			if strings.Contains(strings.ToLower(a.Title), terms[i].Term) {
				terms[i].SampleTitle = a.Title
				terms[i].SampleURL = a.CanonicalURL
				break
			}
		}
	}
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
