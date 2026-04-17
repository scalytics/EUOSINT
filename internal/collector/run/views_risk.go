// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package run

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/config"
)

const (
	viewsRefreshHours     = 168 // weekly
	viewsCountryPageSize  = 500
	viewsGridPageSize     = 1000
	viewsGridTopCountries = 20
	viewsGridMinMean      = 0.5
)

var (
	viewsAPIBase    = "https://api.viewsforecasting.org"
	viewsHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

// viewsCountryRisk is a single country-month forecast row.
type viewsCountryRisk struct {
	ISO       string  `json:"iso"`
	Name      string  `json:"name"`
	CountryID int     `json:"country_id"`
	Year      int     `json:"year"`
	Month     int     `json:"month"`
	SbMean    float64 `json:"sb_mean"`
	SbDich    float64 `json:"sb_dich"`
	NsMean    float64 `json:"ns_mean"`
	NsDich    float64 `json:"ns_dich"`
	OsMean    float64 `json:"os_mean"`
	OsDich    float64 `json:"os_dich"`
}

// viewsGridCell is a single grid-cell forecast row with derived coordinates.
type viewsGridCell struct {
	PgID   int     `json:"pg_id"`
	ISO    string  `json:"iso"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	SbMean float64 `json:"sb_mean"`
	SbDich float64 `json:"sb_dich"`
	NsMean float64 `json:"ns_mean"`
	NsDich float64 `json:"ns_dich"`
	OsMean float64 `json:"os_mean"`
	OsDich float64 `json:"os_dich"`
}

type viewsRiskOutput struct {
	Run       string             `json:"run"`
	FetchedAt string             `json:"fetched_at"`
	MonthID   int                `json:"month_id"`
	Countries []viewsCountryRisk `json:"countries"`
}

type viewsGridOutput struct {
	Run       string          `json:"run"`
	FetchedAt string          `json:"fetched_at"`
	MonthID   int             `json:"month_id"`
	Cells     []viewsGridCell `json:"cells"`
}

// PRIO-GRID: 720 columns (360°/0.5°), rows from south to north.
// pg_id is 1-based: row = (id-1)/720, col = (id-1)%720
func prioGridToLatLng(pgID int) (float64, float64) {
	row := (pgID - 1) / 720
	col := (pgID - 1) % 720
	lat := -90.0 + float64(row)*0.5 + 0.25
	lng := -180.0 + float64(col)*0.5 + 0.25
	return lat, lng
}

// discoverLatestRun picks the most recent fatalities run from the API error response.
func discoverLatestRun(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", viewsAPIBase+"/fatalities003_9999_01_t01/cm/sb?pagesize=1", nil)
	if err != nil {
		return "", err
	}
	resp, err := viewsHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var errResp struct {
		Detail []struct {
			Ctx struct {
				EnumValues []string `json:"enum_values"`
			} `json:"ctx"`
		} `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return "", fmt.Errorf("views: cannot parse run list: %w", err)
	}
	if len(errResp.Detail) == 0 || len(errResp.Detail[0].Ctx.EnumValues) == 0 {
		return "", fmt.Errorf("views: no runs found in API")
	}

	// Find latest fatalities003 run, then fatalities002
	var best string
	for _, prefix := range []string{"fatalities003_", "fatalities002_"} {
		for _, run := range errResp.Detail[0].Ctx.EnumValues {
			if strings.HasPrefix(run, prefix) {
				if run > best {
					best = run
				}
			}
		}
		if best != "" {
			return best, nil
		}
	}
	return "", fmt.Errorf("views: no fatalities run found")
}

type viewsAPIResponse struct {
	NextPage string            `json:"next_page"`
	RowCount int               `json:"row_count"`
	Data     []json.RawMessage `json:"data"`
}

type viewsCMRow struct {
	CountryID int     `json:"country_id"`
	MonthID   int     `json:"month_id"`
	Name      string  `json:"name"`
	IsoAB     string  `json:"isoab"`
	Year      int     `json:"year"`
	Month     int     `json:"month"`
	MainMean  float64 `json:"main_mean"`
	MainDich  float64 `json:"main_dich"`
}

type viewsPGMRow struct {
	PgID     int     `json:"pg_id"`
	MonthID  int     `json:"month_id"`
	MainMean float64 `json:"main_mean"`
	MainDich float64 `json:"main_dich"`
}

func viewsFetchJSON(ctx context.Context, url string) (*viewsAPIResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	resp, err := viewsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("views API %s: status %d", url, resp.StatusCode)
	}
	var result viewsAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("views API %s: decode: %w", url, err)
	}
	return &result, nil
}

// fetchAllPages fetches all pages from a paginated VIEWS endpoint.
func fetchAllPages(ctx context.Context, baseURL string) ([]json.RawMessage, error) {
	var allData []json.RawMessage
	url := baseURL
	for url != "" {
		result, err := viewsFetchJSON(ctx, url)
		if err != nil {
			return allData, err
		}
		allData = append(allData, result.Data...)
		url = strings.TrimSpace(result.NextPage)
	}
	return allData, nil
}

// fetchCountryForecasts fetches country-month forecasts for the 3 violence types.
func fetchCountryForecasts(ctx context.Context, run string, monthID int) ([]viewsCountryRisk, error) {
	types := []struct {
		violence string
		setSb    bool
		setNs    bool
		setOs    bool
	}{
		{"sb", true, false, false},
		{"ns", false, true, false},
		{"os", false, false, true},
	}

	// key = ISO code
	merged := map[string]*viewsCountryRisk{}

	for _, vt := range types {
		url := fmt.Sprintf("%s/%s/cm/%s?pagesize=%d&month=%d", viewsAPIBase, run, vt.violence, viewsCountryPageSize, monthID)
		rows, err := fetchAllPages(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("views cm/%s: %w", vt.violence, err)
		}
		for _, raw := range rows {
			var row viewsCMRow
			if err := json.Unmarshal(raw, &row); err != nil {
				continue
			}
			iso := strings.ToUpper(strings.TrimSpace(row.IsoAB))
			if iso == "" {
				continue
			}
			entry, ok := merged[iso]
			if !ok {
				entry = &viewsCountryRisk{
					ISO:       iso,
					Name:      row.Name,
					CountryID: row.CountryID,
					Year:      row.Year,
					Month:     row.Month,
				}
				merged[iso] = entry
			}
			if vt.setSb {
				entry.SbMean = row.MainMean
				entry.SbDich = row.MainDich
			}
			if vt.setNs {
				entry.NsMean = row.MainMean
				entry.NsDich = row.MainDich
			}
			if vt.setOs {
				entry.OsMean = row.MainMean
				entry.OsDich = row.MainDich
			}
		}
	}

	countries := make([]viewsCountryRisk, 0, len(merged))
	for _, entry := range merged {
		countries = append(countries, *entry)
	}
	sort.Slice(countries, func(i, j int) bool {
		totalI := countries[i].SbMean + countries[i].NsMean + countries[i].OsMean
		totalJ := countries[j].SbMean + countries[j].NsMean + countries[j].OsMean
		return totalI > totalJ
	})
	return countries, nil
}

// fetchGridForecasts fetches PRIO-GRID-month forecasts for the top N countries.
func fetchGridForecasts(ctx context.Context, run string, monthID int, isoCodes []string) ([]viewsGridCell, error) {
	types := []struct {
		violence string
		setSb    bool
		setNs    bool
		setOs    bool
	}{
		{"sb", true, false, false},
		{"ns", false, true, false},
		{"os", false, false, true},
	}

	merged := map[int]*viewsGridCell{}

	for _, vt := range types {
		for _, iso := range isoCodes {
			url := fmt.Sprintf("%s/%s/pgm/%s?iso=%s&pagesize=%d&month=%d",
				viewsAPIBase, run, vt.violence, strings.ToLower(iso), viewsGridPageSize, monthID)
			rows, err := fetchAllPages(ctx, url)
			if err != nil {
				// Non-fatal: some small countries may not have grid data
				continue
			}
			for _, raw := range rows {
				var row viewsPGMRow
				if err := json.Unmarshal(raw, &row); err != nil {
					continue
				}
				entry, ok := merged[row.PgID]
				if !ok {
					lat, lng := prioGridToLatLng(row.PgID)
					entry = &viewsGridCell{PgID: row.PgID, ISO: strings.ToUpper(iso), Lat: lat, Lng: lng}
					merged[row.PgID] = entry
				}
				if vt.setSb {
					entry.SbMean = row.MainMean
					entry.SbDich = row.MainDich
				}
				if vt.setNs {
					entry.NsMean = row.MainMean
					entry.NsDich = row.MainDich
				}
				if vt.setOs {
					entry.OsMean = row.MainMean
					entry.OsDich = row.MainDich
				}
			}
		}
	}

	cells := make([]viewsGridCell, 0, len(merged))
	for _, entry := range merged {
		total := entry.SbMean + entry.NsMean + entry.OsMean
		if total < viewsGridMinMean {
			continue
		}
		// Round to 2 decimal places
		entry.SbMean = math.Round(entry.SbMean*100) / 100
		entry.NsMean = math.Round(entry.NsMean*100) / 100
		entry.OsMean = math.Round(entry.OsMean*100) / 100
		entry.SbDich = math.Round(entry.SbDich*100) / 100
		entry.NsDich = math.Round(entry.NsDich*100) / 100
		entry.OsDich = math.Round(entry.OsDich*100) / 100
		cells = append(cells, *entry)
	}
	sort.Slice(cells, func(i, j int) bool {
		totalI := cells[i].SbMean + cells[i].NsMean + cells[i].OsMean
		totalJ := cells[j].SbMean + cells[j].NsMean + cells[j].OsMean
		return totalI > totalJ
	})
	return cells, nil
}

// currentVIEWSMonthID converts a time to VIEWS month_id (1 = January 1980).
func currentVIEWSMonthID(t time.Time) int {
	return (t.Year()-1980)*12 + int(t.Month())
}

func (r Runner) refreshVIEWSRiskLayer(ctx context.Context, cfg config.Config) error {
	outDir := filepath.Dir(strings.TrimSpace(cfg.OutputPath))
	if strings.TrimSpace(outDir) == "" || outDir == "." {
		outDir = "public"
	}
	countryPath := filepath.Join(outDir, "views-risk.json")
	gridPath := filepath.Join(outDir, "geo", "views-risk-grid.json")

	if !shouldRefreshOutput(countryPath, viewsRefreshHours, time.Now().UTC()) {
		return nil
	}

	fmt.Fprintf(r.stderr, "VIEWS risk layer: discovering latest run...\n")
	run, err := discoverLatestRun(ctx)
	if err != nil {
		return fmt.Errorf("views risk: %w", err)
	}
	fmt.Fprintf(r.stderr, "VIEWS risk layer: using run %s\n", run)

	now := time.Now().UTC()
	currentMonthID := currentVIEWSMonthID(now)
	candidateMonths := []int{currentMonthID, currentMonthID - 1, currentMonthID - 2}
	monthID := currentMonthID
	var countries []viewsCountryRisk
	var lastErr error
	for _, candidate := range candidateMonths {
		if candidate <= 0 {
			continue
		}
		countries, err = fetchCountryForecasts(ctx, run, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		if len(countries) == 0 {
			continue
		}
		monthID = candidate
		break
	}
	if len(countries) == 0 {
		if lastErr != nil {
			return fmt.Errorf("views risk countries: %w", lastErr)
		}
		return fmt.Errorf("views risk countries: no forecast rows for months %v", candidateMonths)
	}
	fmt.Fprintf(r.stderr, "VIEWS risk layer: %d countries fetched\n", len(countries))

	countryOut := viewsRiskOutput{
		Run:       run,
		FetchedAt: now.Format(time.RFC3339),
		MonthID:   monthID,
		Countries: countries,
	}
	countryJSON, err := json.MarshalIndent(countryOut, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(countryPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(countryPath, append(countryJSON, '\n'), 0o644); err != nil {
		return err
	}

	// Pick top N countries by combined fatality forecast for grid-level fetch
	topN := viewsGridTopCountries
	if topN > len(countries) {
		topN = len(countries)
	}
	topISOs := make([]string, 0, topN)
	for i := 0; i < topN; i++ {
		total := countries[i].SbMean + countries[i].NsMean + countries[i].OsMean
		if total < 1.0 {
			break
		}
		topISOs = append(topISOs, countries[i].ISO)
	}

	if len(topISOs) > 0 {
		fmt.Fprintf(r.stderr, "VIEWS risk layer: fetching grid data for %d countries...\n", len(topISOs))
		cells, err := fetchGridForecasts(ctx, run, monthID, topISOs)
		if err != nil {
			fmt.Fprintf(r.stderr, "WARN VIEWS grid fetch: %v\n", err)
		} else {
			fmt.Fprintf(r.stderr, "VIEWS risk layer: %d grid cells fetched\n", len(cells))
			gridOut := viewsGridOutput{
				Run:       run,
				FetchedAt: now.Format(time.RFC3339),
				MonthID:   monthID,
				Cells:     cells,
			}
			gridJSON, err := json.MarshalIndent(gridOut, "", "  ")
			if err == nil {
				if err := os.MkdirAll(filepath.Dir(gridPath), 0o755); err == nil {
					_ = os.WriteFile(gridPath, append(gridJSON, '\n'), 0o644)
				}
			}
		}
	}

	fmt.Fprintf(r.stderr, "VIEWS risk layer refreshed: %d countries, run=%s\n", len(countries), run)
	return nil
}
