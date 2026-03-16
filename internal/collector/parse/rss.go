// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"html"
	"regexp"
	"strings"
)

type FeedItem struct {
	Title     string
	Link      string
	Published string
	Author    string
	Summary   string
	Tags      []string
}

var (
	entryRe        = regexp.MustCompile(`(?is)<entry[\s\S]*?</entry>`)
	itemRe         = regexp.MustCompile(`(?is)<item[\s\S]*?</item>`)
	tagCache       = map[string]*regexp.Regexp{}
	tagValuesCache = map[string]*regexp.Regexp{}
	atomLinkRe     = regexp.MustCompile(`(?is)<link[^>]*rel=["']alternate["'][^>]*>|<link[^>]*>`)
	hrefRe         = regexp.MustCompile(`(?i)href=["']([^"']+)["']`)
	atomAuthorRe   = regexp.MustCompile(`(?is)<author[^>]*>[\s\S]*?<name[^>]*>([\s\S]*?)</name>[\s\S]*?</author>`)
	atomCategoryRe = regexp.MustCompile(`(?is)<category[^>]*term=["']([^"']+)["'][^>]*/?>`)
)

func ParseFeed(xml string) []FeedItem {
	if strings.Contains(xml, "<feed") {
		entries := entryRe.FindAllString(xml, -1)
		items := make([]FeedItem, 0, len(entries))
		for _, entry := range entries {
			items = append(items, FeedItem{
				Title:     getTag(entry, "title"),
				Link:      getAtomLink(entry),
				Published: firstNonEmpty(getTag(entry, "published"), getTag(entry, "updated")),
				Author:    getAuthor(entry),
				Summary:   getSummary(entry),
				Tags:      getCategories(entry),
			})
		}
		return items
	}

	rawItems := itemRe.FindAllString(xml, -1)
	items := make([]FeedItem, 0, len(rawItems))
	for _, item := range rawItems {
		items = append(items, FeedItem{
			Title:     getTag(item, "title"),
			Link:      firstNonEmpty(getTag(item, "link"), getTag(item, "guid")),
			Published: firstNonEmpty(getTag(item, "pubDate"), getTag(item, "dc:date")),
			Author:    getAuthor(item),
			Summary:   getSummary(item),
			Tags:      getCategories(item),
		})
	}
	return items
}

func getTag(block, tag string) string {
	re, ok := tagCache[tag]
	if !ok {
		re = regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `[^>]*>([\s\S]*?)</` + regexp.QuoteMeta(tag) + `>`)
		tagCache[tag] = re
	}
	match := re.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	return decodeXML(match[1])
}

func getTagValues(block, tag string) []string {
	re, ok := tagValuesCache[tag]
	if !ok {
		re = regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `[^>]*>([\s\S]*?)</` + regexp.QuoteMeta(tag) + `>`)
		tagValuesCache[tag] = re
	}
	matches := re.FindAllStringSubmatch(block, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := decodeXML(match[1])
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func getAtomLink(block string) string {
	linkTag := atomLinkRe.FindString(block)
	if linkTag == "" {
		return ""
	}
	match := hrefRe.FindStringSubmatch(linkTag)
	if len(match) < 2 {
		return ""
	}
	return decodeXML(match[1])
}

func getAuthor(block string) string {
	if match := atomAuthorRe.FindStringSubmatch(block); len(match) > 1 {
		return decodeXML(match[1])
	}
	return firstNonEmpty(getTag(block, "author"), getTag(block, "dc:creator"), getTag(block, "creator"))
}

func getSummary(block string) string {
	return firstNonEmpty(
		getTag(block, "description"),
		getTag(block, "summary"),
		getTag(block, "content"),
		getTag(block, "content:encoded"),
	)
}

func getCategories(block string) []string {
	out := getTagValues(block, "category")
	matches := atomCategoryRe.FindAllStringSubmatch(block, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value := decodeXML(match[1])
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func decodeXML(value string) string {
	return strings.TrimSpace(html.UnescapeString(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
