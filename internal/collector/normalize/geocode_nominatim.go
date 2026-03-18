// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// NominatimResult is a geocoding result from OpenStreetMap Nominatim.
type NominatimResult struct {
	Lat         float64
	Lng         float64
	CountryCode string
	DisplayName string
}

// NominatimClient queries the OSM Nominatim API for geocoding.
// It enforces the 1 request/second rate limit for the public instance.
type NominatimClient struct {
	baseURL   string
	userAgent string
	client    *http.Client

	mu       sync.Mutex
	lastCall time.Time

	// Simple in-memory cache to avoid re-querying the same place name.
	cacheMu sync.RWMutex
	cache   map[string]*nominatimCacheEntry
}

type nominatimCacheEntry struct {
	result NominatimResult
	ok     bool
}

type nominatimAPIResponse struct {
	Lat         string `json:"lat"`
	Lon         string `json:"lon"`
	DisplayName string `json:"display_name"`
	Address     struct {
		CountryCode string `json:"country_code"`
	} `json:"address"`
}

// NewNominatimClient creates a Nominatim geocoding client.
// baseURL defaults to the public OSM Nominatim if empty.
func NewNominatimClient(baseURL string, userAgent string) *NominatimClient {
	if baseURL == "" {
		baseURL = "https://nominatim.openstreetmap.org"
	}
	if userAgent == "" {
		userAgent = "EUOSINTBot/1.0 (https://www.scalytics.io; ops@scalytics.io)"
	}
	return &NominatimClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: userAgent,
		client:    &http.Client{Timeout: 10 * time.Second},
		cache:     make(map[string]*nominatimCacheEntry, 256),
	}
}

// Geocode looks up a place name and returns coordinates.
func (n *NominatimClient) Geocode(ctx context.Context, query string, countryCode string) (NominatimResult, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return NominatimResult{}, false
	}

	cacheKey := strings.ToLower(query) + "|" + strings.ToUpper(countryCode)
	n.cacheMu.RLock()
	if entry, ok := n.cache[cacheKey]; ok {
		n.cacheMu.RUnlock()
		return entry.result, entry.ok
	}
	n.cacheMu.RUnlock()

	// Rate limit: 1 req/sec for public Nominatim.
	n.mu.Lock()
	since := time.Since(n.lastCall)
	if since < time.Second {
		time.Sleep(time.Second - since)
	}
	n.lastCall = time.Now()
	n.mu.Unlock()

	params := url.Values{
		"q":               {query},
		"format":          {"json"},
		"limit":           {"1"},
		"addressdetails":  {"1"},
		"accept-language": {"en"},
	}
	if cc := strings.TrimSpace(countryCode); cc != "" {
		params.Set("countrycodes", strings.ToLower(cc))
	}

	reqURL := n.baseURL + "/search?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}
	req.Header.Set("User-Agent", n.userAgent)

	resp, err := n.client.Do(req)
	if err != nil {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}

	var results []nominatimAPIResponse
	if err := json.Unmarshal(body, &results); err != nil || len(results) == 0 {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}

	r := results[0]
	lat, errLat := strconv.ParseFloat(r.Lat, 64)
	lng, errLng := strconv.ParseFloat(r.Lon, 64)
	if errLat != nil || errLng != nil {
		n.cacheNegative(cacheKey)
		return NominatimResult{}, false
	}

	result := NominatimResult{
		Lat:         lat,
		Lng:         lng,
		CountryCode: strings.ToUpper(r.Address.CountryCode),
		DisplayName: r.DisplayName,
	}

	n.cacheMu.Lock()
	n.cache[cacheKey] = &nominatimCacheEntry{result: result, ok: true}
	n.cacheMu.Unlock()

	return result, true
}

func (n *NominatimClient) cacheNegative(key string) {
	n.cacheMu.Lock()
	n.cache[key] = &nominatimCacheEntry{ok: false}
	n.cacheMu.Unlock()
}

// CacheStats returns the number of cached entries (for diagnostics).
func (n *NominatimClient) CacheStats() (total int, hits int) {
	n.cacheMu.RLock()
	defer n.cacheMu.RUnlock()
	total = len(n.cache)
	for _, e := range n.cache {
		if e.ok {
			hits++
		}
	}
	return total, hits
}
