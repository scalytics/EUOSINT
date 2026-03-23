// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package zonebriefllm

import (
	"strings"
	"time"

	"github.com/scalytics/euosint/internal/collector/vet"
)

const (
	SystemPrompt            = "You are an OSINT analyst. Return plain text only. Facts only. Neutral tone. No fluff. No speculation."
	MaxHistoricalWords      = 80
	MaxCurrentAnalysisWords = 60
	MaxRecentHeadlines      = 10
)

type PromptConfig struct {
	ZoneLabel                       string
	Context                         string
	AsOfDate                        time.Time
	IncludeHistoricalCoverageNote   bool
	IncludeRecentHeadlinesNote      bool
	IncludeWeakEvidenceFallbackNote bool
}

func HistoricalMessages(cfg PromptConfig) []vet.Message {
	user := "Short current intelligence summary about conflict zone " + cfg.ZoneLabel + " in max 80 words and current analysis in max 60 words.\nNow return only the historic summary block.\nConstraints: factual only, no bullets, no disclaimers, no filler."
	if cfg.IncludeHistoricalCoverageNote {
		user += "\nCover all major conflicts listed, not just the current one."
	}
	user += "\n" + cfg.Context
	return []vet.Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "user", Content: user},
	}
}

func CurrentAnalysisMessages(cfg PromptConfig) []vet.Message {
	user := "Short current intelligence summary about conflict zone " + cfg.ZoneLabel + " in max 80 words and current analysis in max 60 words.\nNow return only the current analysis block.\nConstraints: factual only, no bullets, no disclaimers, no filler.\nFocus only on current dynamics (roughly last 6-12 months): momentum, intensity direction, territorial/control shifts, and near-term operational outlook.\nDo NOT repeat conflict start date or cumulative death totals from historical summary."
	if cfg.IncludeRecentHeadlinesNote {
		user += "\nIncorporate the recent alert headlines if provided — they represent real-time intelligence from live OSINT feeds."
	}
	if cfg.IncludeWeakEvidenceFallbackNote {
		user += "\nIf recent evidence is weak, give a cautious best-available assessment from the provided context."
	}
	user += "\nAs-of date: " + cfg.AsOfDate.UTC().Format("2006-01-02") + "\n" + cfg.Context
	return []vet.Message{
		{Role: "system", Content: SystemPrompt},
		{Role: "user", Content: user},
	}
}

func AppendRecentHeadlines(baseContext string, recentHeadlines []string, maxHeadlines int) string {
	if len(recentHeadlines) == 0 {
		return baseContext
	}
	limit := len(recentHeadlines)
	if maxHeadlines > 0 && limit > maxHeadlines {
		limit = maxHeadlines
	}
	return baseContext + "\n\nRecent alert headlines from live feeds (last 48h):\n- " + strings.Join(recentHeadlines[:limit], "\n- ")
}

func LimitWords(text string, maxWords int) string {
	if maxWords <= 0 {
		return ""
	}
	words := strings.Fields(text)
	if len(words) <= maxWords {
		return strings.TrimSpace(text)
	}
	return strings.Join(words[:maxWords], " ")
}
