// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/scalytics/euosint/internal/collector/model"
)

// ---------- entity dictionary (loaded once from terror_actor_aliases.json) ----------

type entityDict struct {
	// longest-first sorted phrases → canonical name
	phrases []entityPhrase
}

type entityPhrase struct {
	tokens    []string // lowercased words of the alias
	canonical string
}

var (
	globalEntityDict *entityDict
	entityDictOnce   sync.Once
)

func loadEntityDict() *entityDict {
	entityDictOnce.Do(func() {
		globalEntityDict = buildEntityDict()
	})
	return globalEntityDict
}

func buildEntityDict() *entityDict {
	type aliasGroup struct {
		Canonical string   `json:"canonical"`
		Aliases   []string `json:"aliases"`
	}

	paths := []string{
		"registry/terror_actor_aliases.json",
		"/app/registry/terror_actor_aliases.json",
	}

	var groups []aliasGroup
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(body, &groups); err != nil {
			continue
		}
		break
	}

	dict := &entityDict{}
	for _, g := range groups {
		allAliases := append(g.Aliases, strings.ToLower(g.Canonical))
		for _, alias := range allAliases {
			tokens := tokenize(alias)
			if len(tokens) == 0 {
				continue
			}
			dict.phrases = append(dict.phrases, entityPhrase{
				tokens:    tokens,
				canonical: g.Canonical,
			})
		}
	}

	// Sort longest first so multi-word aliases match before single-word ones.
	sort.Slice(dict.phrases, func(i, j int) bool {
		return len(dict.phrases[i].tokens) > len(dict.phrases[j].tokens)
	})
	return dict
}

// extractEntities scans tokens left-to-right, greedily matching the longest
// known alias phrase. Returns (canonical entity names, remaining tokens).
// Matched positions are consumed so "IS" in "the IS claimed" becomes
// "Islamic State" instead of being dropped as a stopword.
func (d *entityDict) extractEntities(tokens []string) (entities []string, remainder []string) {
	consumed := make([]bool, len(tokens))
	seen := map[string]bool{}

	for _, phrase := range d.phrases {
		pLen := len(phrase.tokens)
		for i := 0; i <= len(tokens)-pLen; i++ {
			if consumed[i] {
				continue
			}
			match := true
			for k := 0; k < pLen; k++ {
				if consumed[i+k] || tokens[i+k] != phrase.tokens[k] {
					match = false
					break
				}
			}
			if match {
				for k := 0; k < pLen; k++ {
					consumed[i+k] = true
				}
				if !seen[phrase.canonical] {
					entities = append(entities, phrase.canonical)
					seen[phrase.canonical] = true
				}
			}
		}
	}

	for i, tok := range tokens {
		if !consumed[i] {
			remainder = append(remainder, tok)
		}
	}
	return entities, remainder
}

// ---------- tokenization & stopword filtering ----------

// Common English stopwords. We do NOT include "is" here — that's handled by
// entity extraction (IS = Islamic State). Only truly meaningless words.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "as": true, "into": true,
	"that": true, "this": true, "it": true, "its": true, "be": true,
	"are": true, "was": true, "were": true, "been": true, "being": true,
	"has": true, "have": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"not": true, "no": true, "nor": true, "so": true, "if": true,
	"up": true, "out": true, "about": true, "than": true, "after": true,
	"before": true, "over": true, "under": true, "between": true,
	"through": true, "during": true, "against": true,
	"says": true, "said": true, "report": true, "reports": true,
	"new": true, "also": true, "more": true, "other": true,
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var tokens []string
	var buf strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(r)
		} else if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		tokens = append(tokens, buf.String())
	}
	return tokens
}

func removeStopwords(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if !stopwords[t] && len(t) > 1 {
			out = append(out, t)
		}
	}
	return out
}

// ---------- fingerprint generation ----------

// contentFingerprint builds a comparable token set from an alert title.
// Entity aliases are resolved to canonical names first (so "ISIS", "ISIL",
// "Daesh", "IS" all become "Islamic State"). Remaining tokens are lowercased
// and stopword-filtered.
// Returns a sorted, deduplicated token list used for Jaccard comparison.
func contentFingerprint(alert model.Alert, dict *entityDict) []string {
	tokens := tokenize(alert.Title)

	// Step 1: extract known entities (greedy longest-match).
	entities, remainder := dict.extractEntities(tokens)

	// Step 2: remove stopwords from the non-entity remainder.
	remainder = removeStopwords(remainder)

	// Step 3: add category and country as context tokens to reduce false positives.
	if alert.Category != "" {
		remainder = append(remainder, "cat:"+alert.Category)
	}
	if alert.EventCountryCode != "" {
		remainder = append(remainder, "cc:"+strings.ToLower(alert.EventCountryCode))
	}

	// Step 4: merge entities (as "entity:CanonicalName") + remaining tokens.
	all := make(map[string]bool, len(entities)+len(remainder))
	for _, e := range entities {
		all["entity:"+e] = true
	}
	for _, t := range remainder {
		all[t] = true
	}

	sorted := make([]string, 0, len(all))
	for t := range all {
		sorted = append(sorted, t)
	}
	sort.Strings(sorted)
	return sorted
}

// ---------- Jaccard similarity ----------

func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := make(map[string]bool, len(a))
	for _, t := range a {
		setA[t] = true
	}
	setB := make(map[string]bool, len(b))
	for _, t := range b {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// ---------- cross-source dedup ----------

const (
	// Minimum Jaccard similarity to consider two alerts as covering the same event.
	fingerprintSimilarityThreshold = 0.65
	// Maximum time gap between alerts to consider them as the same event.
	fingerprintTimeWindowHours = 24
	// Minimum token count for a fingerprint to be meaningful.
	minFingerprintTokens = 3
)

type fingerprintedAlert struct {
	alert  model.Alert
	tokens []string
	ts     time.Time
}

// crossSourceDedup groups alerts from different sources that cover the same
// event, keeping only the highest-scoring version from each group.
// Returns (kept alerts, count of suppressed cross-source duplicates).
func crossSourceDedup(alerts []model.Alert) ([]model.Alert, int) {
	dict := loadEntityDict()

	// Build fingerprints for all alerts.
	fps := make([]fingerprintedAlert, 0, len(alerts))
	var passthrough []model.Alert

	for _, alert := range alerts {
		tokens := contentFingerprint(alert, dict)
		if len(tokens) < minFingerprintTokens {
			passthrough = append(passthrough, alert)
			continue
		}
		ts := parseAlertTime(alert)
		fps = append(fps, fingerprintedAlert{alert: alert, tokens: tokens, ts: ts})
	}

	if len(fps) == 0 {
		return alerts, 0
	}

	// Sort by timestamp so we process oldest first (earlier alerts anchor groups).
	sort.Slice(fps, func(i, j int) bool { return fps[i].ts.Before(fps[j].ts) })

	// Greedy clustering: each alert joins the first cluster it matches,
	// or starts a new cluster. O(n*k) where k = number of clusters.
	type cluster struct {
		members []fingerprintedAlert
		anchor  fingerprintedAlert // first member, used for time window
	}
	var clusters []cluster

	for _, fp := range fps {
		joined := false
		for ci := range clusters {
			anchor := clusters[ci].anchor

			// Time window check.
			if math.Abs(fp.ts.Sub(anchor.ts).Hours()) > fingerprintTimeWindowHours {
				continue
			}

			// Must be from a different source to count as cross-source.
			// (Within-source dupes are already handled by earlier stages.)
			sameSourceExists := false
			for _, m := range clusters[ci].members {
				if m.alert.SourceID == fp.alert.SourceID {
					sameSourceExists = true
					break
				}
			}
			if sameSourceExists {
				continue
			}

			// Jaccard similarity check.
			if jaccardSimilarity(fp.tokens, anchor.tokens) >= fingerprintSimilarityThreshold {
				clusters[ci].members = append(clusters[ci].members, fp)
				joined = true
				break
			}
		}
		if !joined {
			clusters = append(clusters, cluster{
				members: []fingerprintedAlert{fp},
				anchor:  fp,
			})
		}
	}

	// From each cluster, keep the highest-scoring alert.
	kept := append([]model.Alert{}, passthrough...)
	suppressed := 0
	for _, c := range clusters {
		if len(c.members) == 1 {
			kept = append(kept, c.members[0].alert)
			continue
		}
		sort.Slice(c.members, func(i, j int) bool {
			si := alertScore(c.members[i].alert)
			sj := alertScore(c.members[j].alert)
			if si != sj {
				return si > sj
			}
			// Tie-break: prefer earlier (more authoritative first report).
			return c.members[i].ts.Before(c.members[j].ts)
		})
		kept = append(kept, c.members[0].alert)
		suppressed += len(c.members) - 1
	}
	return kept, suppressed
}

func parseAlertTime(alert model.Alert) time.Time {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, alert.FirstSeen); err == nil {
			return t
		}
	}
	return time.Now()
}
