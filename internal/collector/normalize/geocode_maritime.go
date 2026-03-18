// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ── Tier 0a: Explicit coordinate extraction ─────────────────────────
//
// Maritime/military reports often contain literal coordinates like:
//   35°54'N 14°31'E
//   12°03.5'N 045°02.1'E
//   N35.9 E14.5
//   35.9N, 14.5E

// coordDMSRe matches DMS-style coordinates: 35°54'N 14°31'E or 35°54'30"N 14°31'15"E
var coordDMSRe = regexp.MustCompile(
	`(\d{1,3})\s*[°]\s*(\d{1,2})\s*['′]\s*(\d{1,2}(?:\.\d+)?)?\s*[\"″]?\s*([NSns])\s*[,/\s]+` +
		`(\d{1,3})\s*[°]\s*(\d{1,2})\s*['′]\s*(\d{1,2}(?:\.\d+)?)?\s*[\"″]?\s*([EWew])`)

// coordDecMinRe matches decimal-minute style: 12°03.5'N 045°02.1'E
var coordDecMinRe = regexp.MustCompile(
	`(\d{1,3})\s*[°]\s*(\d{1,2}(?:\.\d+)?)\s*['′]?\s*([NSns])\s*[,/\s]+` +
		`(\d{1,3})\s*[°]\s*(\d{1,2}(?:\.\d+)?)\s*['′]?\s*([EWew])`)

// coordDecimalRe matches plain decimal: 35.9N 14.5E or N35.9 E14.5
var coordDecimalRe = regexp.MustCompile(
	`([NSns])\s*(-?\d{1,3}(?:\.\d+)?)\s*[,/\s]+([EWew])\s*(-?\d{1,3}(?:\.\d+)?)` +
		`|(-?\d{1,3}(?:\.\d+)?)\s*([NSns])\s*[,/\s]+(-?\d{1,3}(?:\.\d+)?)\s*([EWew])`)

// coordDegDirRe matches USGS-style: 16.560°N 46.550°W
var coordDegDirRe = regexp.MustCompile(
	`(-?\d{1,3}(?:\.\d+)?)\s*°\s*([NSns])\s*[,/\s]+(-?\d{1,3}(?:\.\d+)?)\s*°\s*([EWew])`)

// ExtractCoordinates tries to find explicit lat/lng coordinates in text.
// Returns the first valid pair found, or (0,0,false).
func ExtractCoordinates(text string) (lat, lng float64, ok bool) {
	// DMS: 35°54'30"N 14°31'15"E
	if m := coordDMSRe.FindStringSubmatch(text); m != nil {
		latD, _ := strconv.ParseFloat(m[1], 64)
		latM, _ := strconv.ParseFloat(m[2], 64)
		latS := 0.0
		if m[3] != "" {
			latS, _ = strconv.ParseFloat(m[3], 64)
		}
		lat = latD + latM/60 + latS/3600
		if strings.ToUpper(m[4]) == "S" {
			lat = -lat
		}
		lngD, _ := strconv.ParseFloat(m[5], 64)
		lngM, _ := strconv.ParseFloat(m[6], 64)
		lngS := 0.0
		if m[7] != "" {
			lngS, _ = strconv.ParseFloat(m[7], 64)
		}
		lng = lngD + lngM/60 + lngS/3600
		if strings.ToUpper(m[8]) == "W" {
			lng = -lng
		}
		if validCoords(lat, lng) {
			return lat, lng, true
		}
	}

	// Decimal minutes: 12°03.5'N 045°02.1'E
	if m := coordDecMinRe.FindStringSubmatch(text); m != nil {
		latD, _ := strconv.ParseFloat(m[1], 64)
		latM, _ := strconv.ParseFloat(m[2], 64)
		lat = latD + latM/60
		if strings.ToUpper(m[3]) == "S" {
			lat = -lat
		}
		lngD, _ := strconv.ParseFloat(m[4], 64)
		lngM, _ := strconv.ParseFloat(m[5], 64)
		lng = lngD + lngM/60
		if strings.ToUpper(m[6]) == "W" {
			lng = -lng
		}
		if validCoords(lat, lng) {
			return lat, lng, true
		}
	}

	// Degree-direction: 16.560°N 46.550°W (USGS earthquake feeds)
	if m := coordDegDirRe.FindStringSubmatch(text); m != nil {
		lat, _ = strconv.ParseFloat(m[1], 64)
		if strings.ToUpper(m[2]) == "S" {
			lat = -lat
		}
		lng, _ = strconv.ParseFloat(m[3], 64)
		if strings.ToUpper(m[4]) == "W" {
			lng = -lng
		}
		if validCoords(lat, lng) {
			return lat, lng, true
		}
	}

	// Decimal: 35.9N 14.5E or N35.9 E14.5
	if m := coordDecimalRe.FindStringSubmatch(text); m != nil {
		if m[1] != "" {
			// N35.9 E14.5 form
			lat, _ = strconv.ParseFloat(m[2], 64)
			if strings.ToUpper(m[1]) == "S" {
				lat = -lat
			}
			lng, _ = strconv.ParseFloat(m[4], 64)
			if strings.ToUpper(m[3]) == "W" {
				lng = -lng
			}
		} else {
			// 35.9N 14.5E form
			lat, _ = strconv.ParseFloat(m[5], 64)
			if strings.ToUpper(m[6]) == "S" {
				lat = -lat
			}
			lng, _ = strconv.ParseFloat(m[7], 64)
			if strings.ToUpper(m[8]) == "W" {
				lng = -lng
			}
		}
		if validCoords(lat, lng) {
			return lat, lng, true
		}
	}

	return 0, 0, false
}

func validCoords(lat, lng float64) bool {
	return math.Abs(lat) <= 90 && math.Abs(lng) <= 180 && (lat != 0 || lng != 0)
}

// ── Tier 0b: Maritime region lookup ─────────────────────────────────
//
// Maps well-known maritime zones, straits, seas, and gulfs to representative
// coordinates. These names appear frequently in maritime security reports
// but are not in city gazetteers.

type maritimeRegion struct {
	Name string
	Lat  float64
	Lng  float64
}

var maritimeRegions = []maritimeRegion{
	// Piracy hotspots
	{"Gulf of Aden", 12.0, 45.0},
	{"Gulf of Guinea", 3.0, 3.0},
	{"Malacca Strait", 2.5, 101.0},
	{"Strait of Malacca", 2.5, 101.0},
	{"Singapore Strait", 1.2, 104.0},
	{"Sulu Sea", 7.5, 120.5},
	{"Celebes Sea", 3.0, 123.0},
	{"South China Sea", 14.0, 114.0},

	// Strategic chokepoints
	{"Strait of Hormuz", 26.5, 56.3},
	{"Bab el-Mandeb", 12.6, 43.3},
	{"Bab al-Mandab", 12.6, 43.3},
	{"Suez Canal", 30.5, 32.3},
	{"Panama Canal", 9.1, -79.7},
	{"Strait of Gibraltar", 35.96, -5.5},
	{"Strait of Dover", 51.0, 1.5},
	{"English Channel", 50.2, -1.0},
	{"Bosphorus", 41.12, 29.05},
	{"Dardanelles", 40.2, 26.4},
	{"Turkish Straits", 41.0, 29.0},
	{"Strait of Messina", 38.2, 15.6},
	{"Strait of Taiwan", 24.0, 119.5},
	{"Taiwan Strait", 24.0, 119.5},
	{"Strait of Sicily", 37.0, 11.5},

	// Seas and oceans
	{"Mediterranean Sea", 35.0, 18.0},
	{"Red Sea", 20.0, 38.5},
	{"Arabian Sea", 15.0, 65.0},
	{"Persian Gulf", 26.0, 52.0},
	{"Arabian Gulf", 26.0, 52.0},
	{"Black Sea", 43.0, 35.0},
	{"Caspian Sea", 41.0, 51.0},
	{"Baltic Sea", 58.0, 20.0},
	{"North Sea", 56.0, 3.0},
	{"Norwegian Sea", 67.0, 3.0},
	{"Barents Sea", 74.0, 36.0},
	{"East China Sea", 30.0, 125.0},
	{"Yellow Sea", 36.0, 123.0},
	{"Sea of Japan", 40.0, 135.0},
	{"Bay of Bengal", 15.0, 88.0},
	{"Andaman Sea", 10.0, 96.0},
	{"Coral Sea", -18.0, 155.0},
	{"Aegean Sea", 38.5, 25.0},
	{"Adriatic Sea", 43.0, 15.5},
	{"Tyrrhenian Sea", 40.0, 12.0},
	{"Ionian Sea", 38.0, 19.0},
	{"Ligurian Sea", 43.5, 8.5},
	{"Sea of Azov", 46.0, 36.5},
	{"Gulf of Oman", 24.5, 58.5},

	// Key shipping corridors
	{"Indian Ocean", -10.0, 70.0},
	{"Horn of Africa", 10.0, 50.0},
	{"Cape of Good Hope", -34.4, 18.5},
	{"Mozambique Channel", -17.0, 42.0},
	{"Gulf of Mexico", 25.0, -90.0},
	{"Caribbean Sea", 15.0, -75.0},
	{"Gulf of Thailand", 9.5, 101.0},

	// European maritime areas
	{"Bay of Biscay", 45.0, -5.0},
	{"Celtic Sea", 50.0, -8.0},
	{"Irish Sea", 53.5, -5.0},
	{"Skagerrak", 58.0, 9.5},
	{"Kattegat", 57.0, 11.5},
}

// maritimeIndex maps lowercased region names to their coordinates. Built at init.
var maritimeIndex map[string]*maritimeRegion

func init() {
	maritimeIndex = make(map[string]*maritimeRegion, len(maritimeRegions))
	for i := range maritimeRegions {
		r := &maritimeRegions[i]
		maritimeIndex[strings.ToLower(r.Name)] = r
	}
}

// MatchMaritimeRegion scans text for known maritime region names and returns
// the best match (rightmost, longest). Returns the region's representative
// coordinates or false if no maritime region is found.
func MatchMaritimeRegion(text string) (lat, lng float64, regionName string, ok bool) {
	lower := strings.ToLower(text)
	bestPos := -1
	bestLen := 0
	var bestRegion *maritimeRegion

	for key, r := range maritimeIndex {
		idx := strings.LastIndex(lower, key)
		if idx < 0 {
			continue
		}
		endPos := idx + len(key)
		if endPos > bestPos || (endPos == bestPos && len(key) > bestLen) {
			bestPos = endPos
			bestLen = len(key)
			bestRegion = r
		}
	}
	if bestRegion != nil {
		return bestRegion.Lat, bestRegion.Lng, bestRegion.Name, true
	}
	return 0, 0, "", false
}
