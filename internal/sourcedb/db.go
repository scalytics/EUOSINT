// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package sourcedb

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/scalytics/euosint/internal/collector/model"
)

//go:embed schema.sql
var schemaSQL string

var initMu sync.Mutex

type DB struct {
	sql *sql.DB
}

type CandidateInput struct {
	DiscoveredURL string
	DiscoveredVia string
	Status        string
	LanguageCode  string
	Category      string
	AuthorityType string
	Country       string
	CountryCode   string
	Notes         string
}

type SourceRunInput struct {
	SourceID      string
	RunStartedAt  string
	RunFinishedAt string
	Status        string
	HTTPStatus    int
	FetchedCount  int
	Error         string
	ErrorClass    string
	ContentHash   string
	ETag          string
	LastModified  string
	Metadata      map[string]any
}

type SourceWatermark struct {
	SourceID          string
	LastRunStartedAt  string
	LastRunFinishedAt string
	LastStatus        string
	LastHTTPStatus    int
	LastFetchedCount  int
	LastContentHash   string
	LastETag          string
	LastModified      string
	LastSuccessAt     string
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create source DB directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite source DB: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite busy_timeout: %w", err)
	}
	return &DB{sql: db}, nil
}

// RawDB returns the underlying *sql.DB for use by subsystems that manage
// their own tables (e.g. trend detection).
func (db *DB) RawDB() *sql.DB {
	return db.sql
}

func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func (db *DB) Init(ctx context.Context) error {
	initMu.Lock()
	defer initMu.Unlock()

	if _, err := db.sql.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("init source DB schema: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE agencies ADD COLUMN level TEXT NOT NULL DEFAULT 'national'`,
		`ALTER TABLE agencies ADD COLUMN mission_tags_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE agencies ADD COLUMN operational_relevance REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE sources ADD COLUMN source_quality REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE sources ADD COLUMN promotion_status TEXT NOT NULL DEFAULT 'candidate'`,
		`ALTER TABLE sources ADD COLUMN rejection_reason TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sources ADD COLUMN is_mirror INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE sources ADD COLUMN preferred_source_rank INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.sql.ExecContext(ctx, stmt); err != nil && !isDuplicateColumnError(err) {
			return fmt.Errorf("migrate source DB schema: %w", err)
		}
	}
	// Backfill live sources from pre-promotion-status databases so existing
	// vetted registry rows remain visible after schema migration.
	if _, err := db.sql.ExecContext(ctx, `
UPDATE sources
SET promotion_status = 'active'
WHERE status IN ('active', '')
  AND promotion_status = 'candidate'
  AND NOT EXISTS (
    SELECT 1
    FROM source_candidates c
    WHERE c.discovered_url = sources.feed_url
  )`); err != nil {
		return fmt.Errorf("backfill source promotion status: %w", err)
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column name")
}

func (db *DB) ImportRegistry(ctx context.Context, registryPath string) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	sources, err := loadRegistryJSON(registryPath)
	if err != nil {
		return err
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin import tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM source_categories`); err != nil {
		return fmt.Errorf("clear source_categories: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agency_category_coverage`); err != nil {
		return fmt.Errorf("clear agency_category_coverage: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sources`); err != nil {
		return fmt.Errorf("clear sources: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agency_aliases`); err != nil {
		return fmt.Errorf("clear agency_aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agencies_fts`); err != nil {
		return fmt.Errorf("clear agencies_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM agencies`); err != nil {
		return fmt.Errorf("clear agencies: %w", err)
	}

	for _, src := range sources {
		agencyID := agencyKey(src.Source)
		if err := upsertAgency(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
		if err := upsertSource(ctx, tx, agencyID, src); err != nil {
			return err
		}
		if err := upsertAgencyCoverage(ctx, tx, agencyID, src.Category); err != nil {
			return err
		}
		if err := upsertAgencyFTS(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit import tx: %w", err)
	}
	return nil
}

func (db *DB) MergeRegistry(ctx context.Context, registryPath string) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	sources, err := loadRegistryJSON(registryPath)
	if err != nil {
		return err
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin merge tx: %w", err)
	}
	defer tx.Rollback()

	for _, src := range sources {
		agencyID := agencyKey(src.Source)
		if err := upsertAgency(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
		if err := upsertSource(ctx, tx, agencyID, src); err != nil {
			return err
		}
		if err := upsertAgencyCoverage(ctx, tx, agencyID, src.Category); err != nil {
			return err
		}
		if err := upsertAgencyFTS(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit merge tx: %w", err)
	}
	return nil
}

func (db *DB) UpsertRegistrySources(ctx context.Context, sources []model.RegistrySource) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert sources tx: %w", err)
	}
	defer tx.Rollback()

	for _, src := range sources {
		agencyID := agencyKey(src.Source)
		if err := upsertAgency(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
		if err := upsertSource(ctx, tx, agencyID, src); err != nil {
			return err
		}
		if err := upsertAgencyCoverage(ctx, tx, agencyID, src.Category); err != nil {
			return err
		}
		if err := upsertAgencyFTS(ctx, tx, agencyID, src.Source); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert sources tx: %w", err)
	}
	return nil
}

func (db *DB) LoadActiveSources(ctx context.Context) ([]model.RegistrySource, error) {
	if err := db.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `
SELECT
  s.id,
  s.type,
  s.fetch_mode,
  s.follow_redirects,
  s.feed_url,
  s.feed_urls_json,
  s.category,
  s.region_tag,
  s.lat,
  s.lng,
  s.max_items,
  s.include_keywords_json,
  s.exclude_keywords_json,
  s.source_quality,
  s.promotion_status,
  s.rejection_reason,
  s.is_mirror,
  s.preferred_source_rank,
  s.reporting_label,
  s.reporting_url,
  s.reporting_phone,
  s.reporting_notes,
  a.authority_name,
  a.language_code,
  a.country,
  a.country_code,
  a.region,
  a.authority_type,
  a.base_url,
  a.scope,
  a.level,
  a.parent_agency_id,
  a.jurisdiction_name,
  a.mission_tags_json,
  a.operational_relevance,
  a.is_curated,
  a.is_high_value
FROM sources s
JOIN agencies a ON a.id = s.agency_id
WHERE s.status IN ('active', '') AND s.promotion_status = 'active'
ORDER BY s.id`)
	if err != nil {
		return nil, fmt.Errorf("query active sources: %w", err)
	}
	defer rows.Close()

	out := make([]model.RegistrySource, 0)
	for rows.Next() {
		var (
			sourceID, sourceType, fetchMode, feedURL, feedURLsJSON, category, regionTag       string
			includeJSON, excludeJSON, promotionStatus, rejectionReason                        string
			reportingLabel, reportingURL, reportingPhone, reportingNotes                      string
			authorityName, languageCode, country, countryCode, region, authorityType, baseURL string
			scope, level, parentAgencyID, jurisdictionName, missionTagsJSON                   string
			followRedirects, isMirror                                                         int
			isCurated, isHighValue                                                            int
			lat, lng, sourceQuality, operationalRelevance                                     float64
			maxItems, preferredSourceRank                                                     int
		)
		if err := rows.Scan(
			&sourceID,
			&sourceType,
			&fetchMode,
			&followRedirects,
			&feedURL,
			&feedURLsJSON,
			&category,
			&regionTag,
			&lat,
			&lng,
			&maxItems,
			&includeJSON,
			&excludeJSON,
			&sourceQuality,
			&promotionStatus,
			&rejectionReason,
			&isMirror,
			&preferredSourceRank,
			&reportingLabel,
			&reportingURL,
			&reportingPhone,
			&reportingNotes,
			&authorityName,
			&languageCode,
			&country,
			&countryCode,
			&region,
			&authorityType,
			&baseURL,
			&scope,
			&level,
			&parentAgencyID,
			&jurisdictionName,
			&missionTagsJSON,
			&operationalRelevance,
			&isCurated,
			&isHighValue,
		); err != nil {
			return nil, fmt.Errorf("scan active source: %w", err)
		}
		var feedURLs, includeKeywords, excludeKeywords, missionTags []string
		if err := decodeJSONStrings(feedURLsJSON, &feedURLs); err != nil {
			return nil, err
		}
		if err := decodeJSONStrings(includeJSON, &includeKeywords); err != nil {
			return nil, err
		}
		if err := decodeJSONStrings(excludeJSON, &excludeKeywords); err != nil {
			return nil, err
		}
		if err := decodeJSONStrings(missionTagsJSON, &missionTags); err != nil {
			return nil, err
		}
		out = append(out, model.RegistrySource{
			Type:            sourceType,
			FetchMode:       emptyToZero(fetchMode),
			FollowRedirects: followRedirects == 1,
			FeedURL:         feedURL,
			FeedURLs:        feedURLs,
			Category:        category,
			RegionTag:       regionTag,
			Lat:             lat,
			Lng:             lng,
			MaxItems:        maxItems,
			IncludeKeywords: includeKeywords,
			ExcludeKeywords: excludeKeywords,
			SourceQuality:   sourceQuality,
			PromotionStatus: promotionStatus,
			RejectionReason: rejectionReason,
			IsMirror:        isMirror == 1,
			PreferredRank:   preferredSourceRank,
			Reporting: model.ReportingMetadata{
				Label: reportingLabel,
				URL:   reportingURL,
				Phone: reportingPhone,
				Notes: reportingNotes,
			},
			Source: model.SourceMetadata{
				SourceID:             sourceID,
				AuthorityName:        authorityName,
				Country:              country,
				CountryCode:          countryCode,
				Region:               region,
				AuthorityType:        authorityType,
				BaseURL:              baseURL,
				Scope:                scope,
				Level:                level,
				ParentAgencyID:       parentAgencyID,
				JurisdictionName:     jurisdictionName,
				MissionTags:          missionTags,
				OperationalRelevance: operationalRelevance,
				IsCurated:            isCurated == 1,
				IsHighValue:          isHighValue == 1,
				LanguageCode:         languageCode,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active sources: %w", err)
	}
	return out, nil
}

func (db *DB) LoadAlerts(ctx context.Context) ([]model.Alert, error) {
	if err := db.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `
SELECT
  alert_id,
  source_id,
  status,
  first_seen,
  last_seen,
  title,
  canonical_url,
  category,
  severity,
  region_tag,
  lat,
  lng,
  freshness_hours,
  source_json,
  reporting_json,
  triage_json
FROM alerts
ORDER BY last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("query alerts: %w", err)
	}
	defer rows.Close()

	out := make([]model.Alert, 0)
	for rows.Next() {
		var (
			alert                                 model.Alert
			sourceJSON, reportingJSON, triageJSON string
		)
		if err := rows.Scan(
			&alert.AlertID,
			&alert.SourceID,
			&alert.Status,
			&alert.FirstSeen,
			&alert.LastSeen,
			&alert.Title,
			&alert.CanonicalURL,
			&alert.Category,
			&alert.Severity,
			&alert.RegionTag,
			&alert.Lat,
			&alert.Lng,
			&alert.FreshnessHours,
			&sourceJSON,
			&reportingJSON,
			&triageJSON,
		); err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		if err := json.Unmarshal([]byte(sourceJSON), &alert.Source); err != nil {
			return nil, fmt.Errorf("decode alert source %s: %w", alert.AlertID, err)
		}
		if strings.TrimSpace(reportingJSON) != "" && reportingJSON != "{}" {
			if err := json.Unmarshal([]byte(reportingJSON), &alert.Reporting); err != nil {
				return nil, fmt.Errorf("decode alert reporting %s: %w", alert.AlertID, err)
			}
		}
		if strings.TrimSpace(triageJSON) != "" && triageJSON != "null" {
			var triage model.Triage
			if err := json.Unmarshal([]byte(triageJSON), &triage); err != nil {
				return nil, fmt.Errorf("decode alert triage %s: %w", alert.AlertID, err)
			}
			alert.Triage = &triage
		}
		out = append(out, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alerts: %w", err)
	}
	return out, nil
}

func (db *DB) UpsertSourceCandidates(ctx context.Context, candidates []CandidateInput) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin candidate upsert tx: %w", err)
	}
	defer tx.Rollback()

	for _, candidate := range candidates {
		discoveredURL := strings.TrimSpace(candidate.DiscoveredURL)
		if discoveredURL == "" {
			continue
		}
		status := strings.TrimSpace(candidate.Status)
		if status == "" {
			status = "candidate"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO source_candidates (
  discovered_url, discovered_via, status, language_code, category, authority_type,
  country, country_code, checked_at, notes
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
ON CONFLICT(discovered_url) DO UPDATE SET
  discovered_via = excluded.discovered_via,
  status = excluded.status,
  language_code = excluded.language_code,
  category = excluded.category,
  authority_type = excluded.authority_type,
  country = excluded.country,
  country_code = excluded.country_code,
  checked_at = CURRENT_TIMESTAMP,
  notes = excluded.notes
`, discoveredURL, strings.TrimSpace(candidate.DiscoveredVia), status, strings.TrimSpace(candidate.LanguageCode), strings.TrimSpace(candidate.Category), strings.TrimSpace(candidate.AuthorityType), strings.TrimSpace(candidate.Country), strings.ToUpper(strings.TrimSpace(candidate.CountryCode)), strings.TrimSpace(candidate.Notes)); err != nil {
			return fmt.Errorf("upsert source candidate %s: %w", discoveredURL, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit candidate upsert tx: %w", err)
	}
	return nil
}

func (db *DB) RecordSourceRun(ctx context.Context, in SourceRunInput) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	sourceID := strings.TrimSpace(in.SourceID)
	if sourceID == "" {
		return nil
	}
	startedAt := strings.TrimSpace(in.RunStartedAt)
	if startedAt == "" {
		startedAt = nowRFC3339()
	}
	finishedAt := strings.TrimSpace(in.RunFinishedAt)
	if finishedAt == "" {
		finishedAt = startedAt
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "unknown"
	}
	if in.FetchedCount < 0 {
		in.FetchedCount = 0
	}
	metaJSON := "{}"
	if len(in.Metadata) > 0 {
		if raw, err := json.Marshal(in.Metadata); err == nil {
			metaJSON = string(raw)
		}
	}

	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source run tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO source_runs (
  source_id, run_started_at, run_finished_at, status, http_status, fetched_count,
  error, error_class, content_hash, etag, last_modified, metadata_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, sourceID, startedAt, finishedAt, status, nullableInt(in.HTTPStatus), in.FetchedCount, strings.TrimSpace(in.Error), strings.TrimSpace(in.ErrorClass), strings.TrimSpace(in.ContentHash), strings.TrimSpace(in.ETag), strings.TrimSpace(in.LastModified), metaJSON); err != nil {
		return fmt.Errorf("insert source run %s: %w", sourceID, err)
	}

	lastSuccessAt := ""
	if status == "ok" || (in.HTTPStatus >= 200 && in.HTTPStatus < 300) {
		lastSuccessAt = finishedAt
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO source_watermarks (
  source_id, last_run_started_at, last_run_finished_at, last_status, last_http_status,
  last_fetched_count, last_content_hash, last_etag, last_modified, last_success_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_id) DO UPDATE SET
  last_run_started_at = excluded.last_run_started_at,
  last_run_finished_at = excluded.last_run_finished_at,
  last_status = excluded.last_status,
  last_http_status = excluded.last_http_status,
  last_fetched_count = excluded.last_fetched_count,
  last_content_hash = excluded.last_content_hash,
  last_etag = excluded.last_etag,
  last_modified = excluded.last_modified,
  last_success_at = CASE
    WHEN excluded.last_success_at != '' THEN excluded.last_success_at
    ELSE source_watermarks.last_success_at
  END,
  updated_at = CURRENT_TIMESTAMP
`, sourceID, startedAt, finishedAt, status, nullableInt(in.HTTPStatus), in.FetchedCount, strings.TrimSpace(in.ContentHash), strings.TrimSpace(in.ETag), strings.TrimSpace(in.LastModified), lastSuccessAt); err != nil {
		return fmt.Errorf("upsert source watermark %s: %w", sourceID, err)
	}

	lastErr := strings.TrimSpace(in.Error)
	lastErrClass := strings.TrimSpace(in.ErrorClass)
	lastOKAt := ""
	if lastSuccessAt != "" {
		lastOKAt = lastSuccessAt
		lastErr = ""
		lastErrClass = ""
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE sources
SET last_http_status = ?,
    last_ok_at = CASE
      WHEN ? != '' THEN ?
      ELSE last_ok_at
    END,
    last_error = ?,
    last_error_class = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?
`, nullableInt(in.HTTPStatus), lastOKAt, lastOKAt, lastErr, lastErrClass, sourceID); err != nil {
		return fmt.Errorf("update source run fields %s: %w", sourceID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source run tx: %w", err)
	}
	return nil
}

func (db *DB) GetSourceWatermark(ctx context.Context, sourceID string) (SourceWatermark, bool, error) {
	if err := db.Init(ctx); err != nil {
		return SourceWatermark{}, false, err
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return SourceWatermark{}, false, nil
	}
	var out SourceWatermark
	var httpStatus sql.NullInt64
	err := db.sql.QueryRowContext(ctx, `
SELECT source_id, last_run_started_at, last_run_finished_at, last_status, last_http_status,
       last_fetched_count, last_content_hash, last_etag, last_modified, last_success_at
FROM source_watermarks
WHERE source_id = ?
`, sourceID).Scan(
		&out.SourceID,
		&out.LastRunStartedAt,
		&out.LastRunFinishedAt,
		&out.LastStatus,
		&httpStatus,
		&out.LastFetchedCount,
		&out.LastContentHash,
		&out.LastETag,
		&out.LastModified,
		&out.LastSuccessAt,
	)
	if err == sql.ErrNoRows {
		return SourceWatermark{}, false, nil
	}
	if err != nil {
		return SourceWatermark{}, false, fmt.Errorf("query source watermark %s: %w", sourceID, err)
	}
	if httpStatus.Valid {
		out.LastHTTPStatus = int(httpStatus.Int64)
	}
	return out, true, nil
}

// GetAllWatermarks loads every source watermark into a map keyed by source ID.
// Used at sweep start so each fetch can check whether content has changed.
func (db *DB) GetAllWatermarks(ctx context.Context) (map[string]*SourceWatermark, error) {
	if err := db.Init(ctx); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `
SELECT source_id, last_run_started_at, last_run_finished_at, last_status, last_http_status,
       last_fetched_count, last_content_hash, last_etag, last_modified, last_success_at
FROM source_watermarks
`)
	if err != nil {
		return nil, fmt.Errorf("query all watermarks: %w", err)
	}
	defer rows.Close()
	out := make(map[string]*SourceWatermark)
	for rows.Next() {
		var wm SourceWatermark
		var httpStatus sql.NullInt64
		if err := rows.Scan(
			&wm.SourceID,
			&wm.LastRunStartedAt,
			&wm.LastRunFinishedAt,
			&wm.LastStatus,
			&httpStatus,
			&wm.LastFetchedCount,
			&wm.LastContentHash,
			&wm.LastETag,
			&wm.LastModified,
			&wm.LastSuccessAt,
		); err != nil {
			return nil, fmt.Errorf("scan watermark row: %w", err)
		}
		if httpStatus.Valid {
			wm.LastHTTPStatus = int(httpStatus.Int64)
		}
		out[wm.SourceID] = &wm
	}
	return out, rows.Err()
}

func (db *DB) SaveAlerts(ctx context.Context, alerts []model.Alert) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert save tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM alerts`); err != nil {
		return fmt.Errorf("clear alerts: %w", err)
	}
	for _, alert := range alerts {
		sourceJSON, _ := json.Marshal(alert.Source)
		reportingJSON, _ := json.Marshal(alert.Reporting)
		triageJSON, _ := json.Marshal(alert.Triage)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO alerts (
  alert_id, source_id, status, first_seen, last_seen, title, canonical_url,
  category, severity, region_tag, lat, lng, freshness_hours, source_json,
  reporting_json, triage_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(alert_id) DO UPDATE SET
  source_id = excluded.source_id,
  status = excluded.status,
  first_seen = excluded.first_seen,
  last_seen = excluded.last_seen,
  title = excluded.title,
  canonical_url = excluded.canonical_url,
  category = excluded.category,
  severity = excluded.severity,
  region_tag = excluded.region_tag,
  lat = excluded.lat,
  lng = excluded.lng,
  freshness_hours = excluded.freshness_hours,
  source_json = excluded.source_json,
  reporting_json = excluded.reporting_json,
  triage_json = excluded.triage_json
`, alert.AlertID, alert.SourceID, alert.Status, alert.FirstSeen, alert.LastSeen, alert.Title, alert.CanonicalURL, alert.Category, alert.Severity, alert.RegionTag, alert.Lat, alert.Lng, alert.FreshnessHours, string(sourceJSON), string(reportingJSON), string(triageJSON)); err != nil {
			return fmt.Errorf("upsert alert %s: %w", alert.AlertID, err)
		}
	}
	// Rebuild FTS index.
	if _, err := tx.ExecContext(ctx, `DELETE FROM alerts_fts`); err != nil {
		return fmt.Errorf("clear alerts_fts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO alerts_fts (alert_id, title, canonical_url, category, severity, region_tag,
  source_authority, source_country, source_country_code)
SELECT a.alert_id, a.title, a.canonical_url, a.category, a.severity, a.region_tag,
  json_extract(a.source_json, '$.authority_name'),
  json_extract(a.source_json, '$.country'),
  json_extract(a.source_json, '$.country_code')
FROM alerts a
`); err != nil {
		return fmt.Errorf("rebuild alerts_fts: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert save tx: %w", err)
	}
	return nil
}

// CorpusScores computes BM25-based distinctiveness scores for all alerts
// in the FTS index. Each alert's title is matched against the full corpus
// — alerts with rare, distinctive terms score higher than those with
// commonly occurring words. Returns a map of alert_id → normalised score (0–1).
//
// Must be called after SaveAlerts (which rebuilds the FTS index).
func (db *DB) CorpusScores(ctx context.Context) (map[string]float64, error) {
	// Collect all alert IDs and titles.
	rows, err := db.sql.QueryContext(ctx, `SELECT alert_id, title FROM alerts WHERE status = 'active'`)
	if err != nil {
		return nil, fmt.Errorf("load alerts for corpus scoring: %w", err)
	}
	type entry struct {
		id    string
		title string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.title); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan alert for corpus scoring: %w", err)
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return map[string]float64{}, nil
	}

	// Score each alert by matching its title against the corpus.
	// BM25 returns negative values (more negative = more relevant in FTS5).
	rawScores := make(map[string]float64, len(entries))
	var minRaw, maxRaw float64
	first := true

	for _, e := range entries {
		// Build FTS5 query from title words. Quote each word for exact matching.
		query := buildCorpusQuery(e.title)
		if query == "" {
			continue
		}
		var rank float64
		err := db.sql.QueryRowContext(ctx, `
SELECT bm25(alerts_fts, 0, 10.0, 0, 3.0, 0, 2.0, 2.0, 2.0, 1.0)
FROM alerts_fts
WHERE alerts_fts MATCH ? AND alert_id = ?
`, query, e.id).Scan(&rank)
		if err != nil {
			// No FTS match (e.g. all stopwords) — skip.
			continue
		}

		rawScores[e.id] = rank
		if first {
			minRaw = rank
			maxRaw = rank
			first = false
		} else {
			if rank < minRaw {
				minRaw = rank
			}
			if rank > maxRaw {
				maxRaw = rank
			}
		}
	}

	// Normalise raw BM25 scores to 0–1.
	// BM25 in FTS5 is negative (more negative = better match). We invert so
	// that the most distinctive alerts get the highest score.
	scores := make(map[string]float64, len(rawScores))
	spread := maxRaw - minRaw
	if spread == 0 {
		// All alerts scored equally — give them all 0.5.
		for id := range rawScores {
			scores[id] = 0.5
		}
	} else {
		for id, raw := range rawScores {
			// Invert: minRaw (best) → 1.0, maxRaw (worst) → 0.0
			normalised := (maxRaw - raw) / spread
			scores[id] = math.Round(normalised*1000) / 1000
		}
	}
	return scores, nil
}

// buildCorpusQuery creates an FTS5 query from a title. Extracts significant
// words, quotes them, and joins with OR so the query matches any term.
func buildCorpusQuery(title string) string {
	words := strings.Fields(strings.ToLower(title))
	var parts []string
	seen := map[string]bool{}
	for _, w := range words {
		// Strip punctuation from edges.
		w = strings.Trim(w, ".,;:!?\"'()-–—/\\[]{}|#@$%^&*+=~`<>")
		if len(w) < 3 || corpusQueryStopwords[w] || seen[w] {
			continue
		}
		seen[w] = true
		// Escape double quotes inside the word.
		w = strings.ReplaceAll(w, `"`, `""`)
		parts = append(parts, `"`+w+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " OR ")
}

var corpusQueryStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "was": true,
	"has": true, "its": true, "not": true, "but": true, "all": true,
	"can": true, "will": true, "had": true, "her": true, "his": true,
	"our": true, "out": true, "who": true, "how": true, "new": true,
	"now": true, "this": true, "that": true, "with": true, "from": true,
	"been": true, "have": true, "more": true, "when": true, "they": true,
	"than": true, "what": true, "were": true, "also": true, "into": true,
	"over": true, "such": true, "just": true, "only": true, "very": true,
	"about": true, "after": true, "which": true, "their": true, "there": true,
	"other": true, "being": true, "could": true, "would": true, "should": true,
}

// SearchAlerts performs full-text search against the alerts FTS index.
// The query supports FTS5 syntax: bare words, "quoted phrases", prefix*, AND/OR/NOT.
// Results are ordered by BM25 relevance. Limit 0 means default (100).
func (db *DB) SearchAlerts(ctx context.Context, query string, category string, region string, status string, limit int) ([]model.Alert, error) {
	if limit <= 0 {
		limit = 100
	}

	query = strings.TrimSpace(query)
	if query == "" && category == "" && region == "" {
		return nil, nil
	}

	var (
		rows *sql.Rows
		err  error
	)
	if query != "" {
		// FTS5 match with optional filters on the joined alerts table.
		where := `WHERE alerts_fts MATCH ?`
		args := []any{query}
		if category != "" {
			where += ` AND a.category = ?`
			args = append(args, category)
		}
		if region != "" {
			where += ` AND a.region_tag = ?`
			args = append(args, region)
		}
		if status != "" {
			where += ` AND a.status = ?`
			args = append(args, status)
		}
		args = append(args, limit)
		rows, err = db.sql.QueryContext(ctx, `
SELECT a.alert_id, a.source_id, a.status, a.first_seen, a.last_seen, a.title,
  a.canonical_url, a.category, a.severity, a.region_tag, a.lat, a.lng,
  a.freshness_hours, a.source_json, a.reporting_json, a.triage_json,
  bm25(alerts_fts, 0, 10.0, 0, 3.0, 0, 2.0, 2.0, 2.0, 1.0) AS rank
FROM alerts_fts
JOIN alerts a ON a.alert_id = alerts_fts.alert_id
`+where+`
ORDER BY rank
LIMIT ?`, args...)
	} else {
		// No text query, just filter.
		where := `WHERE 1=1`
		args := []any{}
		if category != "" {
			where += ` AND a.category = ?`
			args = append(args, category)
		}
		if region != "" {
			where += ` AND a.region_tag = ?`
			args = append(args, region)
		}
		if status != "" {
			where += ` AND a.status = ?`
			args = append(args, status)
		}
		args = append(args, limit)
		rows, err = db.sql.QueryContext(ctx, `
SELECT a.alert_id, a.source_id, a.status, a.first_seen, a.last_seen, a.title,
  a.canonical_url, a.category, a.severity, a.region_tag, a.lat, a.lng,
  a.freshness_hours, a.source_json, a.reporting_json, a.triage_json,
  0 AS rank
FROM alerts a
`+where+`
ORDER BY a.last_seen DESC
LIMIT ?`, args...)
	}
	if err != nil {
		return nil, fmt.Errorf("search alerts: %w", err)
	}
	defer rows.Close()

	var results []model.Alert
	for rows.Next() {
		var (
			a          model.Alert
			sourceJSON string
			reportJSON string
			triageJSON string
			rank       float64
		)
		if err := rows.Scan(&a.AlertID, &a.SourceID, &a.Status, &a.FirstSeen, &a.LastSeen,
			&a.Title, &a.CanonicalURL, &a.Category, &a.Severity, &a.RegionTag,
			&a.Lat, &a.Lng, &a.FreshnessHours, &sourceJSON, &reportJSON, &triageJSON, &rank); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		_ = json.Unmarshal([]byte(sourceJSON), &a.Source)
		_ = json.Unmarshal([]byte(reportJSON), &a.Reporting)
		if triageJSON != "null" && triageJSON != "" {
			var t model.Triage
			if json.Unmarshal([]byte(triageJSON), &t) == nil {
				a.Triage = &t
			}
		}
		results = append(results, a)
	}
	return results, rows.Err()
}

func (db *DB) DeactivateSources(ctx context.Context, reasons map[string]string) error {
	if len(reasons) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deactivate tx: %w", err)
	}
	defer tx.Rollback()

	for sourceID, reason := range reasons {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE sources
SET status = 'needs_replacement',
    promotion_status = 'rejected',
    rejection_reason = ?,
    last_error = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ?`, "Dead source: "+strings.TrimSpace(reason), strings.TrimSpace(reason), sourceID); err != nil {
			return fmt.Errorf("deactivate source %s: %w", sourceID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deactivate tx: %w", err)
	}
	return nil
}

func (db *DB) ExportRegistry(ctx context.Context, registryPath string) error {
	rows, err := db.sql.QueryContext(ctx, `
SELECT
  s.id,
  s.type,
  s.fetch_mode,
  s.follow_redirects,
  s.feed_url,
  s.feed_urls_json,
  s.category,
  s.region_tag,
  s.lat,
  s.lng,
  s.max_items,
  s.include_keywords_json,
  s.exclude_keywords_json,
  s.source_quality,
  s.promotion_status,
  s.rejection_reason,
  s.is_mirror,
  s.preferred_source_rank,
  s.reporting_label,
  s.reporting_url,
  s.reporting_phone,
  s.reporting_notes,
  a.id,
  a.authority_name,
  a.language_code,
  a.country,
  a.country_code,
  a.region,
  a.authority_type,
  a.base_url,
  a.scope,
  a.level,
  a.parent_agency_id,
  a.jurisdiction_name,
  a.mission_tags_json,
  a.operational_relevance,
  a.is_curated,
  a.is_high_value
FROM sources s
JOIN agencies a ON a.id = s.agency_id
WHERE s.status IN ('active', 'candidate', '') AND s.promotion_status != 'rejected'
ORDER BY a.region, a.country, a.authority_name, s.id`)
	if err != nil {
		return fmt.Errorf("query registry export: %w", err)
	}
	defer rows.Close()

	exported := make([]model.RegistrySource, 0)
	for rows.Next() {
		var (
			sourceID, sourceType, fetchMode, feedURL, feedURLsJSON, category, regionTag                 string
			includeJSON, excludeJSON, promotionStatus, rejectionReason                                  string
			reportingLabel, reportingURL, reportingPhone, reportingNotes                                string
			agencyID, authorityName, languageCode, country, countryCode, region, authorityType, baseURL string
			scope, level, parentAgencyID, jurisdictionName, missionTagsJSON                             string
			followRedirects, isMirror                                                                   int
			isCurated, isHighValue                                                                      int
			lat, lng, sourceQuality, operationalRelevance                                               float64
			maxItems, preferredSourceRank                                                               int
		)
		if err := rows.Scan(
			&sourceID,
			&sourceType,
			&fetchMode,
			&followRedirects,
			&feedURL,
			&feedURLsJSON,
			&category,
			&regionTag,
			&lat,
			&lng,
			&maxItems,
			&includeJSON,
			&excludeJSON,
			&sourceQuality,
			&promotionStatus,
			&rejectionReason,
			&isMirror,
			&preferredSourceRank,
			&reportingLabel,
			&reportingURL,
			&reportingPhone,
			&reportingNotes,
			&agencyID,
			&authorityName,
			&languageCode,
			&country,
			&countryCode,
			&region,
			&authorityType,
			&baseURL,
			&scope,
			&level,
			&parentAgencyID,
			&jurisdictionName,
			&missionTagsJSON,
			&operationalRelevance,
			&isCurated,
			&isHighValue,
		); err != nil {
			return fmt.Errorf("scan registry export: %w", err)
		}

		var feedURLs, includeKeywords, excludeKeywords, missionTags []string
		if err := decodeJSONStrings(feedURLsJSON, &feedURLs); err != nil {
			return err
		}
		if err := decodeJSONStrings(includeJSON, &includeKeywords); err != nil {
			return err
		}
		if err := decodeJSONStrings(excludeJSON, &excludeKeywords); err != nil {
			return err
		}
		if err := decodeJSONStrings(missionTagsJSON, &missionTags); err != nil {
			return err
		}

		exported = append(exported, model.RegistrySource{
			Type:            sourceType,
			FetchMode:       emptyToZero(fetchMode),
			FollowRedirects: followRedirects == 1,
			FeedURL:         feedURL,
			FeedURLs:        feedURLs,
			Category:        category,
			RegionTag:       regionTag,
			Lat:             lat,
			Lng:             lng,
			MaxItems:        maxItems,
			IncludeKeywords: includeKeywords,
			ExcludeKeywords: excludeKeywords,
			SourceQuality:   sourceQuality,
			PromotionStatus: promotionStatus,
			RejectionReason: rejectionReason,
			IsMirror:        isMirror == 1,
			PreferredRank:   preferredSourceRank,
			Reporting: model.ReportingMetadata{
				Label: reportingLabel,
				URL:   reportingURL,
				Phone: reportingPhone,
				Notes: reportingNotes,
			},
			Source: model.SourceMetadata{
				SourceID:             agencyIDOrSourceID(agencyID, sourceID),
				AuthorityName:        authorityName,
				Country:              country,
				CountryCode:          countryCode,
				Region:               region,
				AuthorityType:        authorityType,
				BaseURL:              baseURL,
				Scope:                scope,
				Level:                level,
				ParentAgencyID:       parentAgencyID,
				JurisdictionName:     jurisdictionName,
				MissionTags:          missionTags,
				OperationalRelevance: operationalRelevance,
				IsCurated:            isCurated == 1,
				IsHighValue:          isHighValue == 1,
				LanguageCode:         languageCode,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate registry export: %w", err)
	}

	sort.Slice(exported, func(i, j int) bool {
		if exported[i].Source.Region != exported[j].Source.Region {
			return exported[i].Source.Region < exported[j].Source.Region
		}
		if exported[i].Source.Country != exported[j].Source.Country {
			return exported[i].Source.Country < exported[j].Source.Country
		}
		return exported[i].Source.SourceID < exported[j].Source.SourceID
	})

	data, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal exported registry: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(registryPath, data, 0o644); err != nil {
		return fmt.Errorf("write exported registry: %w", err)
	}
	return nil
}

func upsertAgency(ctx context.Context, tx *sql.Tx, agencyID string, meta model.SourceMetadata) error {
	missionTagsJSON, _ := json.Marshal(compactStrings(meta.MissionTags...))
	_, err := tx.ExecContext(ctx, `
INSERT INTO agencies (id, authority_name, language_code, country, country_code, region, authority_type, base_url, scope, level, parent_agency_id, jurisdiction_name, mission_tags_json, operational_relevance, is_curated, is_high_value)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  authority_name = excluded.authority_name,
  language_code = excluded.language_code,
  country = excluded.country,
  country_code = excluded.country_code,
  region = excluded.region,
  authority_type = excluded.authority_type,
  base_url = excluded.base_url,
  scope = excluded.scope,
  level = excluded.level,
  parent_agency_id = excluded.parent_agency_id,
  jurisdiction_name = excluded.jurisdiction_name,
  mission_tags_json = excluded.mission_tags_json,
  operational_relevance = excluded.operational_relevance,
  is_curated = excluded.is_curated,
  is_high_value = excluded.is_high_value,
  updated_at = CURRENT_TIMESTAMP
`, agencyID, meta.AuthorityName, meta.LanguageCode, meta.Country, meta.CountryCode, meta.Region, meta.AuthorityType, meta.BaseURL, fallbackScope(meta.Scope), fallbackLevel(meta.Level, meta.Scope), meta.ParentAgencyID, meta.JurisdictionName, string(missionTagsJSON), defaultOperationalRelevance(meta), boolToInt(meta.IsCurated), boolToInt(meta.IsHighValue))
	if err != nil {
		return fmt.Errorf("upsert agency %s: %w", agencyID, err)
	}
	return nil
}

func upsertSource(ctx context.Context, tx *sql.Tx, agencyID string, src model.RegistrySource) error {
	feedURLsJSON, _ := json.Marshal(src.FeedURLs)
	includeJSON, _ := json.Marshal(src.IncludeKeywords)
	excludeJSON, _ := json.Marshal(src.ExcludeKeywords)
	_, err := tx.ExecContext(ctx, `
INSERT INTO sources (
  id, agency_id, language_code, type, fetch_mode, follow_redirects, feed_url, feed_urls_json,
  category, region_tag, lat, lng, max_items, include_keywords_json, exclude_keywords_json,
  source_quality, promotion_status, rejection_reason, is_mirror, preferred_source_rank,
  reporting_label, reporting_url, reporting_phone, reporting_notes, status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active')
ON CONFLICT(id) DO UPDATE SET
  agency_id = excluded.agency_id,
  language_code = excluded.language_code,
  type = excluded.type,
  fetch_mode = excluded.fetch_mode,
  follow_redirects = excluded.follow_redirects,
  feed_url = excluded.feed_url,
  feed_urls_json = excluded.feed_urls_json,
  category = excluded.category,
  region_tag = excluded.region_tag,
  lat = excluded.lat,
  lng = excluded.lng,
  max_items = excluded.max_items,
  include_keywords_json = excluded.include_keywords_json,
  exclude_keywords_json = excluded.exclude_keywords_json,
  source_quality = excluded.source_quality,
  promotion_status = CASE
    WHEN excluded.promotion_status = 'rejected' THEN 'rejected'
    WHEN sources.promotion_status = 'rejected' AND excluded.promotion_status IN ('active', '') THEN 'rejected'
    ELSE excluded.promotion_status
  END,
  rejection_reason = CASE
    WHEN excluded.promotion_status = 'rejected' THEN excluded.rejection_reason
    WHEN sources.promotion_status = 'rejected' AND excluded.promotion_status IN ('active', '') THEN sources.rejection_reason
    ELSE excluded.rejection_reason
  END,
  is_mirror = excluded.is_mirror,
  preferred_source_rank = excluded.preferred_source_rank,
  reporting_label = excluded.reporting_label,
  reporting_url = excluded.reporting_url,
  reporting_phone = excluded.reporting_phone,
  reporting_notes = excluded.reporting_notes,
  status = CASE
    WHEN sources.promotion_status = 'rejected' AND excluded.promotion_status IN ('active', '') THEN sources.status
    ELSE 'active'
  END,
  updated_at = CURRENT_TIMESTAMP
`,
		src.Source.SourceID,
		agencyID,
		src.Source.LanguageCode,
		src.Type,
		src.FetchMode,
		boolToInt(src.FollowRedirects),
		src.FeedURL,
		string(feedURLsJSON),
		src.Category,
		src.RegionTag,
		src.Lat,
		src.Lng,
		src.MaxItems,
		string(includeJSON),
		string(excludeJSON),
		defaultSourceQuality(src),
		defaultPromotionStatus(src),
		strings.TrimSpace(src.RejectionReason),
		boolToInt(src.IsMirror),
		src.PreferredRank,
		src.Reporting.Label,
		src.Reporting.URL,
		src.Reporting.Phone,
		src.Reporting.Notes,
	)
	if err != nil {
		return fmt.Errorf("upsert source %s: %w", src.Source.SourceID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_categories (source_id, category) VALUES (?, ?)`, src.Source.SourceID, src.Category); err != nil {
		return fmt.Errorf("upsert source category %s: %w", src.Source.SourceID, err)
	}
	return nil
}

func upsertAgencyCoverage(ctx context.Context, tx *sql.Tx, agencyID string, category string) error {
	category = strings.TrimSpace(category)
	if agencyID == "" || category == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agency_category_coverage (agency_id, category) VALUES (?, ?)`, agencyID, category); err != nil {
		return fmt.Errorf("upsert agency coverage %s/%s: %w", agencyID, category, err)
	}
	return nil
}

func upsertAgencyFTS(ctx context.Context, tx *sql.Tx, agencyID string, meta model.SourceMetadata) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM agencies_fts WHERE agency_id = ?`, agencyID); err != nil {
		return fmt.Errorf("clear agency fts %s: %w", agencyID, err)
	}
	aliases := strings.Join(compactStrings(meta.SourceID, meta.AuthorityName), " ")
	_, err := tx.ExecContext(ctx, `
INSERT INTO agencies_fts (agency_id, authority_name, aliases, country, country_code, region, authority_type, base_url)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, agencyID, meta.AuthorityName, aliases, meta.Country, meta.CountryCode, meta.Region, meta.AuthorityType, meta.BaseURL)
	if err != nil {
		return fmt.Errorf("upsert agency fts %s: %w", agencyID, err)
	}
	return nil
}

func fallbackScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "national"
	}
	return scope
}

func fallbackLevel(level string, scope string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level != "" {
		return level
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "international", "supranational", "federal", "national", "regional", "local":
		return scope
	default:
		return "national"
	}
}

func defaultOperationalRelevance(meta model.SourceMetadata) float64 {
	if meta.OperationalRelevance > 0 {
		return meta.OperationalRelevance
	}
	score := 0.65
	switch fallbackLevel(meta.Level, meta.Scope) {
	case "international":
		score = 0.95
	case "supranational":
		score = 0.92
	case "federal":
		score = 0.9
	case "national":
		score = 0.82
	case "regional":
		score = 0.45
	case "local":
		score = 0.2
	}
	if meta.IsHighValue {
		score += 0.05
	}
	if meta.IsCurated {
		score += 0.03
	}
	if score > 1 {
		score = 1
	}
	return score
}

func defaultSourceQuality(src model.RegistrySource) float64 {
	if src.SourceQuality > 0 {
		return src.SourceQuality
	}
	score := 0.72
	switch strings.TrimSpace(src.Type) {
	case "rss", "travelwarning-atom":
		score = 0.9
	case "kev-json", "interpol-red-json", "interpol-yellow-json", "travelwarning-json":
		score = 0.95
	case "html-list":
		score = 0.62
	}
	if src.Source.IsCurated {
		score += 0.03
	}
	if src.Source.IsHighValue {
		score += 0.03
	}
	if src.IsMirror {
		score -= 0.1
	}
	if score > 1 {
		score = 1
	}
	return score
}

func defaultPromotionStatus(src model.RegistrySource) string {
	status := strings.ToLower(strings.TrimSpace(src.PromotionStatus))
	if status != "" {
		return status
	}
	level := fallbackLevel(src.Source.Level, src.Source.Scope)
	if level == "local" {
		return "rejected"
	}
	if level == "regional" && !src.Source.IsCurated && !src.Source.IsHighValue {
		return "validated"
	}
	return "active"
}

func decodeJSONStrings(raw string, target *[]string) error {
	if strings.TrimSpace(raw) == "" {
		*target = nil
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("decode string array %q: %w", raw, err)
	}
	return nil
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func emptyToZero(v string) string {
	return strings.TrimSpace(v)
}

func agencyIDOrSourceID(agencyID, sourceID string) string {
	if strings.TrimSpace(agencyID) != "" {
		return agencyID
	}
	return sourceID
}

func agencyKey(meta model.SourceMetadata) string {
	base := strings.ToLower(strings.TrimSpace(meta.AuthorityName))
	base = strings.ReplaceAll(base, "&", " and ")
	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, base)
	base = strings.Trim(base, "-")
	base = strings.Join(strings.FieldsFunc(base, func(r rune) bool { return r == '-' }), "-")
	if base == "" {
		base = strings.ToLower(strings.TrimSpace(meta.SourceID))
	}
	if code := strings.ToLower(strings.TrimSpace(meta.CountryCode)); code != "" {
		return base + "-" + code
	}
	return base
}

func loadRegistryJSON(path string) ([]model.RegistrySource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}
	var raw []model.RegistrySource
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode registry %s: %w", path, err)
	}
	return raw, nil
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
