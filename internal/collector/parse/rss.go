// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package parse

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

type FeedItem struct {
	Title     string
	Link      string
	Published string
	Author    string
	Summary   string
	Tags      []string
	Lat       float64 // from <georss:point> if present
	Lng       float64 // from <georss:point> if present
}

var (
	entryRe        = regexp.MustCompile(`(?is)<entry[\s\S]*?</entry>`)
	itemRe         = regexp.MustCompile(`(?is)<item[\s\S]*?</item>`)
	tagCache       sync.Map
	tagValuesCache sync.Map
	georssPtRe     = regexp.MustCompile(`(?is)<georss:point[^>]*>\s*(-?[\d.]+)\s+(-?[\d.]+)\s*</georss:point>`)
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
			lat, lng := getGeoRSSPoint(entry)
			items = append(items, FeedItem{
				Title:     getTag(entry, "title"),
				Link:      getAtomLink(entry),
				Published: firstNonEmpty(getTag(entry, "published"), getTag(entry, "updated")),
				Author:    getAuthor(entry),
				Summary:   getSummary(entry),
				Tags:      getCategories(entry),
				Lat:       lat,
				Lng:       lng,
			})
		}
		return items
	}

	rawItems := itemRe.FindAllString(xml, -1)
	items := make([]FeedItem, 0, len(rawItems))
	for _, item := range rawItems {
		lat, lng := getGeoRSSPoint(item)
		items = append(items, FeedItem{
			Title:     getTag(item, "title"),
			Link:      firstNonEmpty(getTag(item, "link"), getTag(item, "guid")),
			Published: firstNonEmpty(getTag(item, "pubDate"), getTag(item, "dc:date")),
			Author:    getAuthor(item),
			Summary:   getSummary(item),
			Tags:      getCategories(item),
			Lat:       lat,
			Lng:       lng,
		})
	}
	return items
}

func getTag(block, tag string) string {
	cached, ok := tagCache.Load(tag)
	if !ok {
		compiled := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `[^>]*>([\s\S]*?)</` + regexp.QuoteMeta(tag) + `>`)
		cached, _ = tagCache.LoadOrStore(tag, compiled)
	}
	re := cached.(*regexp.Regexp)
	match := re.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	return decodeXML(match[1])
}

func getTagValues(block, tag string) []string {
	cached, ok := tagValuesCache.Load(tag)
	if !ok {
		compiled := regexp.MustCompile(`(?is)<` + regexp.QuoteMeta(tag) + `[^>]*>([\s\S]*?)</` + regexp.QuoteMeta(tag) + `>`)
		cached, _ = tagValuesCache.LoadOrStore(tag, compiled)
	}
	re := cached.(*regexp.Regexp)
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

// getGeoRSSPoint extracts lat/lng from <georss:point>lat lng</georss:point>.
func getGeoRSSPoint(block string) (lat, lng float64) {
	m := georssPtRe.FindStringSubmatch(block)
	if len(m) < 3 {
		return 0, 0
	}
	lat, err1 := strconv.ParseFloat(m[1], 64)
	lng, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return lat, lng
}
