// Copyright 2026 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package discover

import (
	"net/url"
	"strings"
)

var localEntityTerms = []string{
	"municipal",
	"municipality",
	"city of ",
	"county ",
	"sheriff",
	"borough",
	"township",
	"village",
	"metropolitan police",
	"local police",
	"police department",
}

var genericNewsroomTerms = []string{
	"newsroom",
	"press office",
	"media centre",
	"media center",
	"communications office",
}

func passesDiscoveryHygiene(name string, website string, authorityType string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	authorityType = strings.ToLower(strings.TrimSpace(authorityType))
	if name == "" {
		return false
	}
	for _, term := range localEntityTerms {
		if strings.Contains(name, term) {
			return false
		}
	}
	if authorityType == "police" {
		for _, term := range genericNewsroomTerms {
			if strings.Contains(name, term) {
				return false
			}
		}
	}
	if hostLooksLocal(website) {
		return false
	}
	return true
}

func hostLooksLocal(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	return strings.HasPrefix(host, "city.") ||
		strings.HasPrefix(host, "county.") ||
		strings.HasPrefix(host, "police.") ||
		strings.Contains(host, ".city.") ||
		strings.Contains(host, ".county.") ||
		strings.Contains(host, ".municipal.")
}
