// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	tgMessageRe  = regexp.MustCompile(`(?is)<div class="tgme_widget_message_wrap[^"]*"[^>]*data-post="([^"]+)"[\s\S]*?</div>\s*</div>\s*</div>\s*</div>`)
	tgTextRe     = regexp.MustCompile(`(?is)<div class="tgme_widget_message_text[^"]*"[^>]*>([\s\S]*?)</div>`)
	tgDatetimeRe = regexp.MustCompile(`(?is)<time[^>]*datetime="([^"]+)"`)
)

// ParseTelegram extracts messages from a Telegram t.me/s/<channel> HTML page.
// Each message becomes a FeedItem with:
//   - Title: first ~200 chars of the text content
//   - Link:  https://t.me/<channel>/<msgid>
//   - Published: datetime from <time> tag
func ParseTelegram(body string, channel string) []FeedItem {
	blocks := tgMessageRe.FindAllStringSubmatch(body, -1)
	if len(blocks) == 0 {
		return parseTelegramFallback(body, channel)
	}

	seen := make(map[string]struct{}, len(blocks))
	out := make([]FeedItem, 0, len(blocks))
	for _, match := range blocks {
		dataPost := match[1] // e.g. "channel/12345"
		if _, ok := seen[dataPost]; ok {
			continue
		}
		seen[dataPost] = struct{}{}

		block := match[0]
		text := extractTgText(block)
		if len(text) < 8 {
			continue
		}

		title := text
		if len(title) > 200 {
			title = title[:200] + "…"
		}

		link := fmt.Sprintf("https://t.me/%s", dataPost)
		published := ""
		if dtMatch := tgDatetimeRe.FindStringSubmatch(block); len(dtMatch) > 1 {
			published = dtMatch[1]
		}

		out = append(out, FeedItem{
			Title:     title,
			Link:      link,
			Published: published,
		})
	}
	return out
}

// parseTelegramFallback uses a simpler pattern when the full wrapper regex
// doesn't match (Telegram occasionally tweaks markup).
func parseTelegramFallback(body string, channel string) []FeedItem {
	textBlocks := tgTextRe.FindAllStringSubmatch(body, -1)
	out := make([]FeedItem, 0, len(textBlocks))
	for i, match := range textBlocks {
		text := StripHTML(match[1])
		if len(text) < 8 {
			continue
		}
		title := text
		if len(title) > 200 {
			title = title[:200] + "…"
		}
		out = append(out, FeedItem{
			Title: title,
			Link:  fmt.Sprintf("https://t.me/%s/%d", channel, i+1),
		})
	}
	return out
}

func extractTgText(block string) string {
	m := tgTextRe.FindStringSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	text := StripHTML(m[1])
	return strings.TrimSpace(text)
}
