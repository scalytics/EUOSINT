package sourcedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/parse"
)

type UCDPLensState struct {
	LensID      string
	Version     string
	StartDate   string
	EndDate     string
	HeadHash    string
	TotalPages  int
	EventCount  int
	RefreshedAt string
}

func (db *DB) LoadUCDPLensEvents(ctx context.Context, lensID string) ([]parse.UCDPItem, error) {
	if err := db.Init(ctx); err != nil {
		return nil, err
	}
	lensID = strings.TrimSpace(lensID)
	if lensID == "" {
		return nil, nil
	}
	rows, err := db.sql.QueryContext(ctx, `
SELECT title, link, published, summary, tags_json,
       lat, lng, violence_type, fatalities, civilian_deaths,
       country, country_code, region, side_a, side_b, dyad_name, admin1, admin2,
       where_precision, date_precision, event_clarity
FROM ucdp_lens_events
WHERE lens_id = ?
ORDER BY published DESC, event_key ASC
`, lensID)
	if err != nil {
		return nil, fmt.Errorf("load ucdp lens events: %w", err)
	}
	defer rows.Close()

	out := make([]parse.UCDPItem, 0)
	for rows.Next() {
		var item parse.UCDPItem
		var tagsJSON string
		if err := rows.Scan(
			&item.Title,
			&item.Link,
			&item.Published,
			&item.Summary,
			&tagsJSON,
			&item.Lat,
			&item.Lng,
			&item.ViolenceType,
			&item.Fatalities,
			&item.CivilianDeaths,
			&item.Country,
			&item.CountryCode,
			&item.Region,
			&item.SideA,
			&item.SideB,
			&item.DyadName,
			&item.Admin1,
			&item.Admin2,
			&item.WherePrecision,
			&item.DatePrecision,
			&item.EventClarity,
		); err != nil {
			return nil, fmt.Errorf("scan ucdp lens event: %w", err)
		}
		if strings.TrimSpace(tagsJSON) != "" {
			_ = json.Unmarshal([]byte(tagsJSON), &item.Tags)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ucdp lens events: %w", err)
	}
	return out, nil
}

func (db *DB) ReplaceUCDPLensEvents(ctx context.Context, lensID string, items []parse.UCDPItem) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	lensID = strings.TrimSpace(lensID)
	if lensID == "" {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace ucdp lens events tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM ucdp_lens_events WHERE lens_id = ?`, lensID); err != nil {
		return fmt.Errorf("clear ucdp lens events: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO ucdp_lens_events (
  lens_id, event_key, published, lat, lng, where_precision, date_precision, event_clarity,
  country, country_code, region, violence_type, fatalities, civilian_deaths,
  side_a, side_b, dyad_name, admin1, admin2, title, link, summary, tags_json, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare insert ucdp lens events: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range items {
		if err := execUpsertUCDPLensEvent(ctx, stmt, lensID, item, now); err != nil {
			return fmt.Errorf("insert ucdp lens event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace ucdp lens events tx: %w", err)
	}
	return nil
}

func (db *DB) UpsertUCDPLensEvents(ctx context.Context, lensID string, items []parse.UCDPItem) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	lensID = strings.TrimSpace(lensID)
	if lensID == "" || len(items) == 0 {
		return nil
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert ucdp lens events tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO ucdp_lens_events (
  lens_id, event_key, published, lat, lng, where_precision, date_precision, event_clarity,
  country, country_code, region, violence_type, fatalities, civilian_deaths,
  side_a, side_b, dyad_name, admin1, admin2, title, link, summary, tags_json, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(lens_id, event_key) DO UPDATE SET
  published = excluded.published,
  lat = excluded.lat,
  lng = excluded.lng,
  where_precision = excluded.where_precision,
  date_precision = excluded.date_precision,
  event_clarity = excluded.event_clarity,
  country = excluded.country,
  country_code = excluded.country_code,
  region = excluded.region,
  violence_type = excluded.violence_type,
  fatalities = excluded.fatalities,
  civilian_deaths = excluded.civilian_deaths,
  side_a = excluded.side_a,
  side_b = excluded.side_b,
  dyad_name = excluded.dyad_name,
  admin1 = excluded.admin1,
  admin2 = excluded.admin2,
  title = excluded.title,
  link = excluded.link,
  summary = excluded.summary,
  tags_json = excluded.tags_json,
  updated_at = excluded.updated_at
`)
	if err != nil {
		return fmt.Errorf("prepare upsert ucdp lens events: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range items {
		if err := execUpsertUCDPLensEvent(ctx, stmt, lensID, item, now); err != nil {
			return fmt.Errorf("upsert ucdp lens event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert ucdp lens events tx: %w", err)
	}
	return nil
}

func (db *DB) GetUCDPLensState(ctx context.Context, lensID string) (UCDPLensState, bool, error) {
	if err := db.Init(ctx); err != nil {
		return UCDPLensState{}, false, err
	}
	lensID = strings.TrimSpace(lensID)
	if lensID == "" {
		return UCDPLensState{}, false, nil
	}
	var out UCDPLensState
	err := db.sql.QueryRowContext(ctx, `
SELECT lens_id, version, start_date, end_date, head_hash, total_pages, event_count, refreshed_at
FROM ucdp_lens_state
WHERE lens_id = ?
`, lensID).Scan(
		&out.LensID,
		&out.Version,
		&out.StartDate,
		&out.EndDate,
		&out.HeadHash,
		&out.TotalPages,
		&out.EventCount,
		&out.RefreshedAt,
	)
	if err == sql.ErrNoRows {
		return UCDPLensState{}, false, nil
	}
	if err != nil {
		return UCDPLensState{}, false, fmt.Errorf("get ucdp lens state: %w", err)
	}
	return out, true, nil
}

func (db *DB) UpsertUCDPLensState(ctx context.Context, in UCDPLensState) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	in.LensID = strings.TrimSpace(in.LensID)
	if in.LensID == "" {
		return nil
	}
	if strings.TrimSpace(in.RefreshedAt) == "" {
		in.RefreshedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.sql.ExecContext(ctx, `
INSERT INTO ucdp_lens_state (
  lens_id, version, start_date, end_date, head_hash, total_pages, event_count, refreshed_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(lens_id) DO UPDATE SET
  version = excluded.version,
  start_date = excluded.start_date,
  end_date = excluded.end_date,
  head_hash = excluded.head_hash,
  total_pages = excluded.total_pages,
  event_count = excluded.event_count,
  refreshed_at = excluded.refreshed_at,
  updated_at = CURRENT_TIMESTAMP
`, in.LensID, in.Version, in.StartDate, in.EndDate, in.HeadHash, in.TotalPages, in.EventCount, in.RefreshedAt)
	if err != nil {
		return fmt.Errorf("upsert ucdp lens state: %w", err)
	}
	return nil
}

func UCDPEventKey(item parse.UCDPItem) string {
	link := strings.TrimSpace(item.Link)
	if link != "" {
		return link
	}
	return fmt.Sprintf(
		"%s|%s|%.5f|%.5f|%s|%s",
		strings.TrimSpace(item.Title),
		strings.TrimSpace(item.Published),
		item.Lat,
		item.Lng,
		strings.TrimSpace(item.SideA),
		strings.TrimSpace(item.SideB),
	)
}

func execUpsertUCDPLensEvent(ctx context.Context, stmt *sql.Stmt, lensID string, item parse.UCDPItem, now string) error {
	eventKey := UCDPEventKey(item)
	tagsJSON := "[]"
	if len(item.Tags) > 0 {
		if raw, err := json.Marshal(item.Tags); err == nil {
			tagsJSON = string(raw)
		}
	}
	_, err := stmt.ExecContext(
		ctx,
		lensID,
		eventKey,
		item.Published,
		item.Lat,
		item.Lng,
		item.WherePrecision,
		item.DatePrecision,
		item.EventClarity,
		item.Country,
		item.CountryCode,
		item.Region,
		item.ViolenceType,
		item.Fatalities,
		item.CivilianDeaths,
		item.SideA,
		item.SideB,
		item.DyadName,
		item.Admin1,
		item.Admin2,
		item.Title,
		item.Link,
		item.Summary,
		tagsJSON,
		now,
	)
	return err
}
