package sourcedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (db *DB) GetUCDPDatasetCache(ctx context.Context, datasetKey, version string) (string, []byte, bool, error) {
	if err := db.Init(ctx); err != nil {
		return "", nil, false, err
	}
	datasetKey = strings.TrimSpace(datasetKey)
	version = strings.TrimSpace(version)
	if datasetKey == "" || version == "" {
		return "", nil, false, nil
	}
	var headHash string
	var payloadJSON string
	err := db.sql.QueryRowContext(ctx, `
SELECT head_hash, payload_json
FROM ucdp_dataset_cache
WHERE dataset_key = ? AND version = ?
`, datasetKey, version).Scan(&headHash, &payloadJSON)
	if err == sql.ErrNoRows {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("get ucdp dataset cache: %w", err)
	}
	return headHash, []byte(payloadJSON), true, nil
}

func (db *DB) UpsertUCDPDatasetCache(ctx context.Context, datasetKey, version, headHash string, payloadJSON []byte) error {
	if err := db.Init(ctx); err != nil {
		return err
	}
	datasetKey = strings.TrimSpace(datasetKey)
	version = strings.TrimSpace(version)
	if datasetKey == "" || version == "" {
		return nil
	}
	refreshedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := db.sql.ExecContext(ctx, `
INSERT INTO ucdp_dataset_cache (
  dataset_key, version, head_hash, payload_json, refreshed_at, updated_at
)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(dataset_key, version) DO UPDATE SET
  head_hash = excluded.head_hash,
  payload_json = excluded.payload_json,
  refreshed_at = excluded.refreshed_at,
  updated_at = CURRENT_TIMESTAMP
`, datasetKey, version, strings.TrimSpace(headHash), string(payloadJSON), refreshedAt)
	if err != nil {
		return fmt.Errorf("upsert ucdp dataset cache: %w", err)
	}
	return nil
}
