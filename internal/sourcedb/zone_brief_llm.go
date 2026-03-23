package sourcedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ZoneBriefLLM struct {
	CountryID           string
	Title               string
	HistoricalSummary   string
	CurrentAnalysis     string
	HistoricalUpdatedAt string
	AnalysisUpdatedAt   string
}

func (db *DB) GetZoneBriefLLM(ctx context.Context, countryID string) (ZoneBriefLLM, bool, error) {
	if err := db.Init(ctx); err != nil {
		return ZoneBriefLLM{}, false, err
	}
	countryID = strings.TrimSpace(countryID)
	if countryID == "" {
		return ZoneBriefLLM{}, false, nil
	}
	var out ZoneBriefLLM
	err := db.sql.QueryRowContext(ctx, `
SELECT country_id, title, historical_summary, current_analysis, historical_updated_at, analysis_updated_at
FROM zone_brief_llm
WHERE country_id = ?
`, countryID).Scan(
		&out.CountryID,
		&out.Title,
		&out.HistoricalSummary,
		&out.CurrentAnalysis,
		&out.HistoricalUpdatedAt,
		&out.AnalysisUpdatedAt,
	)
	if err == sql.ErrNoRows {
		return ZoneBriefLLM{}, false, nil
	}
	if err != nil {
		return ZoneBriefLLM{}, false, fmt.Errorf("get zone brief llm: %w", err)
	}
	return out, true, nil
}

func (db *DB) UpsertZoneBriefLLM(ctx context.Context, in ZoneBriefLLM) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	in.CountryID = strings.TrimSpace(in.CountryID)
	in.Title = strings.TrimSpace(in.Title)
	if in.CountryID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(in.HistoricalUpdatedAt) == "" && strings.TrimSpace(in.HistoricalSummary) != "" {
		in.HistoricalUpdatedAt = now
	}
	if strings.TrimSpace(in.AnalysisUpdatedAt) == "" && strings.TrimSpace(in.CurrentAnalysis) != "" {
		in.AnalysisUpdatedAt = now
	}
	_, err := db.sql.ExecContext(ctx, `
INSERT INTO zone_brief_llm (
  country_id, title, historical_summary, current_analysis, historical_updated_at, analysis_updated_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(country_id) DO UPDATE SET
  title = excluded.title,
  historical_summary = CASE
    WHEN excluded.historical_summary <> '' THEN excluded.historical_summary
    ELSE zone_brief_llm.historical_summary
  END,
  current_analysis = CASE
    WHEN excluded.current_analysis <> '' THEN excluded.current_analysis
    ELSE zone_brief_llm.current_analysis
  END,
  historical_updated_at = CASE
    WHEN excluded.historical_summary <> '' THEN excluded.historical_updated_at
    ELSE zone_brief_llm.historical_updated_at
  END,
  analysis_updated_at = CASE
    WHEN excluded.current_analysis <> '' THEN excluded.analysis_updated_at
    ELSE zone_brief_llm.analysis_updated_at
  END,
  updated_at = CURRENT_TIMESTAMP
`, in.CountryID, in.Title, in.HistoricalSummary, in.CurrentAnalysis, in.HistoricalUpdatedAt, in.AnalysisUpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert zone brief llm: %w", err)
	}
	return nil
}

func (db *DB) ResetZoneBriefLLM(ctx context.Context) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	_, err := db.sql.ExecContext(ctx, `DELETE FROM zone_brief_llm`)
	if err != nil {
		return fmt.Errorf("reset zone brief llm: %w", err)
	}
	return nil
}
