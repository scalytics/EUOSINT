package sourcedb

import (
	"context"
	"testing"
)

func TestUCDPDatasetCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if _, _, ok, err := db.GetUCDPDatasetCache(ctx, "ged", "26.1"); err != nil || ok {
		t.Fatalf("expected empty cache initially, ok=%v err=%v", ok, err)
	}

	payload := []byte(`{"events":[1,2,3]}`)
	if err := db.UpsertUCDPDatasetCache(ctx, "ged", "26.1", "hash-1", payload); err != nil {
		t.Fatalf("upsert dataset cache: %v", err)
	}

	headHash, gotPayload, ok, err := db.GetUCDPDatasetCache(ctx, "ged", "26.1")
	if err != nil {
		t.Fatalf("get dataset cache: %v", err)
	}
	if !ok {
		t.Fatal("expected dataset cache row")
	}
	if headHash != "hash-1" {
		t.Fatalf("unexpected head hash: %q", headHash)
	}
	if string(gotPayload) != string(payload) {
		t.Fatalf("unexpected payload: %q", string(gotPayload))
	}
}

func TestUCDPDatasetCacheBlankKeyNoop(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)

	if err := db.UpsertUCDPDatasetCache(ctx, " ", "26.1", "hash", []byte(`{}`)); err != nil {
		t.Fatalf("upsert blank key should be noop, err=%v", err)
	}
	if err := db.UpsertUCDPDatasetCache(ctx, "ged", " ", "hash", []byte(`{}`)); err != nil {
		t.Fatalf("upsert blank version should be noop, err=%v", err)
	}
	if _, _, ok, err := db.GetUCDPDatasetCache(ctx, " ", "26.1"); err != nil || ok {
		t.Fatalf("blank get key expected miss; ok=%v err=%v", ok, err)
	}
	if _, _, ok, err := db.GetUCDPDatasetCache(ctx, "ged", " "); err != nil || ok {
		t.Fatalf("blank get version expected miss; ok=%v err=%v", ok, err)
	}
}
