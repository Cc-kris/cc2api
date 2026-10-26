package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/lib/pq"
)

const (
	contentTaxonomyDefaultPath        = "data/content_taxonomy.json"
	contentFeaturedBackfillLock int64 = 219
)

type contentTaxonomyBackfillMarker struct {
	State                    string  `json:"state,omitempty"`
	StartedAt                string  `json:"startedAt,omitempty"`
	CompletedAt              string  `json:"completedAt,omitempty"`
	SnapshotFeaturedIDs      []int64 `json:"snapshotFeaturedArticleIds"`
	SnapshotTaggedArticleIDs []int64 `json:"snapshotTaggedArticleIds"`
	SnapshotLegacyArticleIDs []int64 `json:"snapshotLegacyArticleIds"`
	PreviousFeaturedIDs      []int64 `json:"previousFeaturedArticleIds"`
	TargetFeaturedIDs        []int64 `json:"targetFeaturedArticleIds"`
}

type contentFeaturedSourceRow struct {
	ID        int64
	UpdatedAt time.Time
}

// RunContentFeaturedBackfill runs the one-time v0.4 featured-article migration.
// TASK-035 owns invoking this function during server startup.
func RunContentFeaturedBackfill(ctx context.Context, db *sql.DB) error {
	return RunContentFeaturedBackfillAt(ctx, db, contentTaxonomyDefaultPath)
}

// RunContentFeaturedBackfillAt is the injectable form of RunContentFeaturedBackfill.
// It is intentionally independent of ContentArticleAdminService so startup wiring
// does not depend on the admin HTTP service.
func RunContentFeaturedBackfillAt(ctx context.Context, db *sql.DB, taxonomyPath string) error {
	return runContentFeaturedBackfillAt(ctx, db, taxonomyPath, acquireContentFeaturedBackfillLock, writeContentTaxonomyAtomic)
}

type contentFeaturedLockFunc func(context.Context, *sql.Tx) (func(), error)

func acquireContentFeaturedBackfillLock(ctx context.Context, tx *sql.Tx) (func(), error) {
	var lockResult any
	if err := tx.QueryRowContext(ctx, `SELECT pg_advisory_xact_lock($1)`, contentFeaturedBackfillLock).Scan(&lockResult); err != nil {
		return nil, err
	}
	return func() {}, nil
}

// Lock and writer parameters are narrow test seams for concurrent serialization
// and marker-write failure recovery tests.
func runContentFeaturedBackfillAt(ctx context.Context, db *sql.DB, taxonomyPath string, acquireLock contentFeaturedLockFunc, writeTaxonomy func(string, map[string]json.RawMessage) error) error {
	if db == nil {
		return errors.New("content featured backfill: database is nil")
	}
	if taxonomyPath == "" {
		return errors.New("content featured backfill: taxonomy path is empty")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("content featured backfill: begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	releaseLock, err := acquireLock(ctx, tx)
	if err != nil {
		return fmt.Errorf("content featured backfill: acquire advisory lock: %w", err)
	}
	if releaseLock != nil {
		defer releaseLock()
	}

	// Taxonomy and its idempotence marker are deliberately re-read only after
	// acquiring the transaction-scoped lock. A waiter therefore observes the
	// winner's durable completion marker instead of replaying a stale read.
	raw, taxonomyIDs, marker, err := readContentTaxonomy(taxonomyPath)
	if err != nil {
		return fmt.Errorf("content featured backfill: read taxonomy under lock: %w", err)
	}
	if marker.CompletedAt != "" {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("content featured backfill: commit completed-marker check: %w", err)
		}
		committed = true
		return nil
	}
	if marker.State == "pending" && marker.StartedAt == "" {
		return errors.New("content featured backfill: pending marker has no start time")
	}

	// Re-read all DB source/current state under the same lock and transaction.
	taggedRows, err := queryFeaturedTaggedArticles(ctx, tx)
	if err != nil {
		return fmt.Errorf("content featured backfill: read tagged articles: %w", err)
	}
	legacyRows, err := queryTaxonomyArticles(ctx, tx, taxonomyIDs)
	if err != nil {
		return fmt.Errorf("content featured backfill: read taxonomy articles: %w", err)
	}
	currentIDs, err := queryCurrentFeaturedArticleIDs(ctx, tx)
	if err != nil {
		return fmt.Errorf("content featured backfill: read existing featured articles: %w", err)
	}

	allRows := make(map[int64]contentFeaturedSourceRow, len(taggedRows)+len(legacyRows))
	for _, row := range taggedRows {
		allRows[row.ID] = row
	}
	for _, row := range legacyRows {
		allRows[row.ID] = row
	}

	isPending := marker.State == "pending"
	var targetIDs []int64
	applyPlan := false
	supersededPending := false
	if isPending {
		if marker.TargetFeaturedIDs == nil || marker.PreviousFeaturedIDs == nil {
			return errors.New("content featured backfill: pending marker has incomplete snapshot")
		}
		if !equalArticleIDs(taxonomyIDs, marker.SnapshotFeaturedIDs) {
			return errors.New("content featured backfill: taxonomy changed since pending snapshot")
		}
		if !equalArticleIDs(sourceIDs(legacyRows), marker.SnapshotLegacyArticleIDs) {
			return errors.New("content featured backfill: legacy DB sources changed since pending snapshot")
		}
		switch {
		case equalArticleIDs(currentIDs, marker.TargetFeaturedIDs) && len(taggedRows) == 0:
			// The DB transaction committed before a prior final-marker write failed.
			// This also handles no-op featured plans where previous == target.
			targetIDs = cloneArticleIDs(currentIDs)
		case equalArticleIDs(currentIDs, marker.PreviousFeaturedIDs) && equalArticleIDs(sourceIDs(taggedRows), marker.SnapshotTaggedArticleIDs):
			// The prior DB transaction rolled back (or made no featured_at change);
			// verify its original tag source set before replaying the durable plan.
			targetIDs = cloneArticleIDs(marker.TargetFeaturedIDs)
			applyPlan = !equalArticleIDs(currentIDs, targetIDs)
		case equalArticleIDs(currentIDs, marker.TargetFeaturedIDs):
			// Featured IDs show the target, but tag removal is part of that same
			// transaction; a remaining tag means this is not its committed result.
			return errors.New("content featured backfill: tagged DB sources remain after pending target was applied")
		case equalArticleIDs(currentIDs, marker.PreviousFeaturedIDs):
			return errors.New("content featured backfill: tagged DB sources changed since pending snapshot")
		default:
			// A later manual selection superseded the pending plan. Preserve it
			// and refresh the snapshot to the exact locked DB state being finalized.
			targetIDs = cloneArticleIDs(currentIDs)
			supersededPending = true
		}
	} else if len(currentIDs) > 0 {
		// Existing/manual featured selections always win over legacy sources.
		targetIDs = cloneArticleIDs(currentIDs)
	} else {
		targetIDs = make([]int64, 0, len(allRows))
		for id := range allRows {
			targetIDs = append(targetIDs, id)
		}
		sort.Slice(targetIDs, func(i, j int) bool { return targetIDs[i] > targetIDs[j] })
		if len(targetIDs) > 10 {
			targetIDs = targetIDs[:10]
		}
		applyPlan = len(targetIDs) > 0
	}

	if !isPending {
		marker = contentTaxonomyBackfillMarker{
			State:                    "pending",
			StartedAt:                time.Now().UTC().Format(time.RFC3339Nano),
			SnapshotFeaturedIDs:      append([]int64(nil), taxonomyIDs...),
			SnapshotTaggedArticleIDs: sourceIDs(taggedRows),
			SnapshotLegacyArticleIDs: sourceIDs(legacyRows),
			PreviousFeaturedIDs:      cloneArticleIDs(currentIDs),
			TargetFeaturedIDs:        cloneArticleIDs(targetIDs),
		}
	} else {
		// If manual DB changes superseded an interrupted plan, persist the actual
		// source and featured sets observed under the lock, not stale prior values.
		marker.TargetFeaturedIDs = cloneArticleIDs(targetIDs)
		if supersededPending {
			marker.SnapshotFeaturedIDs = cloneArticleIDs(taxonomyIDs)
			marker.SnapshotTaggedArticleIDs = sourceIDs(taggedRows)
			marker.SnapshotLegacyArticleIDs = sourceIDs(legacyRows)
			marker.PreviousFeaturedIDs = cloneArticleIDs(currentIDs)
		}
	}
	pendingBytes, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("content featured backfill: encode pending marker: %w", err)
	}
	raw["contentV1Migration"] = pendingBytes
	if err := writeTaxonomy(taxonomyPath, raw); err != nil {
		return fmt.Errorf("content featured backfill: persist pending snapshot: %w", err)
	}

	if applyPlan {
		base, parseErr := time.Parse(time.RFC3339Nano, marker.StartedAt)
		if parseErr != nil {
			return fmt.Errorf("content featured backfill: invalid pending start time: %w", parseErr)
		}
		for i, id := range targetIDs {
			candidate := base.Add(-time.Duration(i) * time.Microsecond)
			if _, err := tx.ExecContext(ctx,
				`UPDATE content_articles SET featured_at = $1 WHERE id = $2`, candidate, id); err != nil {
				return fmt.Errorf("content featured backfill: set featured_at for article %d: %w", id, err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE content_articles SET featured_at = NULL WHERE featured_at IS NOT NULL AND NOT (id = ANY($1))`, pq.Array(targetIDs)); err != nil {
			return fmt.Errorf("content featured backfill: clear stale featured_at: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE content_articles SET tags = COALESCE(tags, '[]'::jsonb) - '精选' WHERE tags @> '["精选"]'::jsonb`); err != nil {
		return fmt.Errorf("content featured backfill: clear legacy featured tags: %w", err)
	}
	if err := reconcileContentFeaturedArticles(ctx, tx, targetIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("content featured backfill: commit: %w", err)
	}
	committed = true

	marker.State = "completed"
	marker.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	marker.TargetFeaturedIDs = cloneArticleIDs(targetIDs)
	markerBytes, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("content featured backfill: encode completion marker: %w", err)
	}
	raw["featuredArticleIds"] = json.RawMessage("[]")
	raw["contentV1Migration"] = markerBytes
	if err := writeTaxonomy(taxonomyPath, raw); err != nil {
		return fmt.Errorf("content featured backfill: persist completion marker: %w", err)
	}
	return nil
}

func cloneArticleIDs(ids []int64) []int64 {
	cloned := make([]int64, len(ids))
	copy(cloned, ids)
	return cloned
}

func equalArticleIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readContentTaxonomy(path string) (map[string]json.RawMessage, []int64, contentTaxonomyBackfillMarker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, contentTaxonomyBackfillMarker{}, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, contentTaxonomyBackfillMarker{}, err
	}
	if raw == nil {
		return nil, nil, contentTaxonomyBackfillMarker{}, errors.New("taxonomy JSON must be an object")
	}
	var ids []int64
	if value, ok := raw["featuredArticleIds"]; ok {
		if err := json.Unmarshal(value, &ids); err != nil {
			return nil, nil, contentTaxonomyBackfillMarker{}, fmt.Errorf("featuredArticleIds: %w", err)
		}
	}
	var marker contentTaxonomyBackfillMarker
	if value, ok := raw["contentV1Migration"]; ok {
		if err := json.Unmarshal(value, &marker); err != nil {
			return nil, nil, contentTaxonomyBackfillMarker{}, fmt.Errorf("contentV1Migration: %w", err)
		}
	}
	return raw, ids, marker, nil
}

func sourceIDs(rows []contentFeaturedSourceRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	return ids
}

func queryCurrentFeaturedArticleIDs(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM content_articles
		WHERE featured_at IS NOT NULL
		ORDER BY featured_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func queryFeaturedTaggedArticles(ctx context.Context, tx *sql.Tx) ([]contentFeaturedSourceRow, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(updated_at, NOW())
		FROM content_articles
		WHERE tags @> '["精选"]'::jsonb`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []contentFeaturedSourceRow
	for rows.Next() {
		var row contentFeaturedSourceRow
		if err := rows.Scan(&row.ID, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func queryTaxonomyArticles(ctx context.Context, tx *sql.Tx, ids []int64) ([]contentFeaturedSourceRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(updated_at, NOW())
		FROM content_articles
		WHERE id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []contentFeaturedSourceRow
	for rows.Next() {
		var row contentFeaturedSourceRow
		if err := rows.Scan(&row.ID, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func reconcileContentFeaturedArticles(ctx context.Context, tx *sql.Tx, expected []int64) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, featured_at
		FROM content_articles
		WHERE featured_at IS NOT NULL
		ORDER BY featured_at DESC, id DESC`)
	if err != nil {
		return fmt.Errorf("content featured backfill: reconcile query: %w", err)
	}
	defer rows.Close()
	var actual []int64
	for rows.Next() {
		var id int64
		var featuredAt time.Time
		if err := rows.Scan(&id, &featuredAt); err != nil {
			return fmt.Errorf("content featured backfill: reconcile scan: %w", err)
		}
		actual = append(actual, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("content featured backfill: reconcile rows: %w", err)
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("content featured backfill: reconciliation mismatch: got %v, want %v", actual, expected)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return fmt.Errorf("content featured backfill: reconciliation order mismatch: got %v, want %v", actual, expected)
		}
	}
	return nil
}

func writeContentTaxonomyAtomic(path string, raw map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".content_taxonomy.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
