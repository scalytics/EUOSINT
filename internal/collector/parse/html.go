// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"html"
	"net/url"
	"regexp"
	"strings"
)

var anchorRe = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>([\s\S]*?)</a>`)
var tagStripRe = regexp.MustCompile(`(?is)<[^>]+>`)
var scriptStripRe = regexp.MustCompile(`(?is)<script[\s\S]*?</script>|<style[\s\S]*?</style>`)

func ParseHTMLAnchors(body string, baseURL string) []FeedItem {
	matches := anchorRe.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	out := make([]FeedItem, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		rawHref := strings.TrimSpace(match[1])
		if rawHref == "" || strings.HasPrefix(rawHref, "#") {
			continue
		}
		link, err := url.Parse(rawHref)
		if err != nil {
			continue
		}
		resolved, err := url.Parse(baseURL)
		if err != nil {
			continue
		}
		title := StripHTML(match[2])
		if len(title) < 8 {
			continue
		}
		finalURL := resolved.ResolveReference(link).String()
		if _, ok := seen[finalURL]; ok {
			continue
		}
		seen[finalURL] = struct{}{}
		out = append(out, FeedItem{Title: title, Link: finalURL})
	}
	return out
}

// StripHTML removes script/style tags, strips remaining HTML tags,
// unescapes entities, and normalizes whitespace.
func StripHTML(value string) string {
	value = scriptStripRe.ReplaceAllString(value, " ")
	value = tagStripRe.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}
