// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package noisegate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scalytics/euosint/internal/collector/model"
	"github.com/scalytics/euosint/internal/collector/parse"
)

type Outcome string

const (
	OutcomeKeep      Outcome = "keep"
	OutcomeDowngrade Outcome = "downgrade"
	OutcomeDrop      Outcome = "drop"
)

type CooccurrenceRule struct {
	Name        string   `json:"name"`
	RequiresAny []string `json:"requires_any"`
	WithAny     []string `json:"with_any"`
	Effect      string   `json:"effect"`
}

type Policy struct {
	Version            string             `json:"version"`
	HardBlockTerms     []string           `json:"hard_block_terms"`
	DowngradeTerms     []string           `json:"downgrade_terms"`
	ActionableOverride []string           `json:"actionable_overrides"`
	CooccurrenceRules  []CooccurrenceRule `json:"cooccurrence_rules"`
}

type Decision struct {
	Outcome            Outcome
	PolicyVersion      string
	BlockScore         float64
	NoiseScore         float64
	ActionabilityScore float64
	Reasons            []string
}

type Engine struct {
	policy Policy
}

func Load(path string) (*Engine, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read noise policy: %w", err)
	}
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode noise policy: %w", err)
	}
	normalizePolicy(&p)
	if p.Version == "" {
		p.Version = "v0"
	}
	return &Engine{policy: p}, nil
}

func (e *Engine) Version() string {
	if e == nil {
		return ""
	}
	return e.policy.Version
}

func (e *Engine) Evaluate(source model.RegistrySource, item parse.FeedItem) Decision {
	if e == nil {
		return Decision{Outcome: OutcomeKeep}
	}
	text := strings.ToLower(strings.Join([]string{
		item.Title,
		item.Summary,
		item.Author,
		strings.Join(item.Tags, " "),
		item.Link,
		source.Category,
		source.Source.AuthorityType,
	}, " "))
	reasons := make([]string, 0, 8)

	blockHits := countMatches(text, e.policy.HardBlockTerms)
	noiseHits := countMatches(text, e.policy.DowngradeTerms)
	actionHits := countMatches(text, e.policy.ActionableOverride)

	for _, rule := range e.policy.CooccurrenceRules {
		if len(rule.RequiresAny) == 0 || len(rule.WithAny) == 0 {
			continue
		}
		if !containsAny(text, rule.RequiresAny) || !containsAny(text, rule.WithAny) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rule.Effect)) {
		case "keep", "boost_actionability":
			actionHits += 2
			reasons = append(reasons, "cooccurrence:"+rule.Name)
		case "downgrade":
			noiseHits++
			reasons = append(reasons, "cooccurrence-downgrade:"+rule.Name)
		case "drop", "block":
			blockHits++
			reasons = append(reasons, "cooccurrence-block:"+rule.Name)
		}
	}

	blockScore := hitScore(blockHits, 2, 0.0)
	noiseScore := hitScore(noiseHits, 3, 0.0)
	actionabilityScore := hitScore(actionHits, 3, 0.15)

	if source.Source.IsCurated {
		actionabilityScore += 0.08
	}
	if source.Source.IsHighValue {
		actionabilityScore += 0.08
	}
	actionabilityScore = clamp01(actionabilityScore)

	outcome := OutcomeKeep
	switch {
	case blockScore >= 0.9 && actionabilityScore < 0.35:
		outcome = OutcomeDrop
		reasons = append(reasons, "hard-block-threshold")
	case noiseScore >= 0.6 && actionabilityScore < 0.55:
		outcome = OutcomeDowngrade
		reasons = append(reasons, "downgrade-threshold")
	default:
		if actionHits > 0 {
			reasons = append(reasons, "actionable-override")
		}
	}

	return Decision{
		Outcome:            outcome,
		PolicyVersion:      e.policy.Version,
		BlockScore:         round3(blockScore),
		NoiseScore:         round3(noiseScore),
		ActionabilityScore: round3(actionabilityScore),
		Reasons:            reasons,
	}
}

func normalizePolicy(p *Policy) {
	p.HardBlockTerms = normalizeTerms(p.HardBlockTerms)
	p.DowngradeTerms = normalizeTerms(p.DowngradeTerms)
	p.ActionableOverride = normalizeTerms(p.ActionableOverride)
	for i := range p.CooccurrenceRules {
		p.CooccurrenceRules[i].Name = strings.TrimSpace(p.CooccurrenceRules[i].Name)
		p.CooccurrenceRules[i].RequiresAny = normalizeTerms(p.CooccurrenceRules[i].RequiresAny)
		p.CooccurrenceRules[i].WithAny = normalizeTerms(p.CooccurrenceRules[i].WithAny)
		p.CooccurrenceRules[i].Effect = strings.ToLower(strings.TrimSpace(p.CooccurrenceRules[i].Effect))
	}
}

func normalizeTerms(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func countMatches(text string, terms []string) int {
	hits := 0
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(term, " ") {
			if strings.Contains(text, term) {
				hits++
			}
			continue
		}
		if containsWholeWord(text, term) {
			hits++
		}
	}
	return hits
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(term, " ") {
			if strings.Contains(text, term) {
				return true
			}
			continue
		}
		if containsWholeWord(text, term) {
			return true
		}
	}
	return false
}

func containsWholeWord(hay string, needle string) bool {
	start := 0
	for {
		idx := strings.Index(hay[start:], needle)
		if idx < 0 {
			return false
		}
		idx += start
		leftOK := idx == 0 || !isWordChar(rune(hay[idx-1]))
		end := idx + len(needle)
		rightOK := end >= len(hay) || !isWordChar(rune(hay[end]))
		if leftOK && rightOK {
			return true
		}
		start = idx + len(needle)
		if start >= len(hay) {
			return false
		}
	}
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func hitScore(hits int, saturateAt int, fallback float64) float64 {
	if hits <= 0 || saturateAt <= 0 {
		return fallback
	}
	return clamp01(float64(hits) / float64(saturateAt))
}

func round3(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
