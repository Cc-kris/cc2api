package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func writeTestTaxonomy(t *testing.T, value string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "content_taxonomy.json")
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func expectAdvisoryLock(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
		WithArgs(int64(contentFeaturedBackfillLock)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
}

func TestRunContentFeaturedBackfillReReadsCompletedMarkerAfterLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := writeTestTaxonomy(t, `{"featuredArticleIds":[9]}`)
	mock.ExpectBegin()
	mock.ExpectCommit()

	err = runContentFeaturedBackfillAt(context.Background(), db, path, func(_ context.Context, _ *sql.Tx) (func(), error) {
		// Simulate another runner publishing completion while this runner waits
		// for the transaction-scoped lock.
		if err := os.WriteFile(path, []byte(`{"featuredArticleIds":[],"contentV1Migration":{"state":"completed","completedAt":"2026-10-26T00:00:00Z"}}`), 0600); err != nil {
			return nil, err
		}
		return func() {}, nil
	}, writeContentTaxonomyAtomic)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunContentFeaturedBackfillConcurrentWaitersSerializeAndReReadMarker(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := writeTestTaxonomy(t, `{"featuredArticleIds":[9]}`)
	mock.ExpectBegin()
	mock.ExpectBegin()
	mock.ExpectCommit()
	mock.ExpectCommit()

	var mutex sync.Mutex
	var count atomic.Int32
	arrivals := make(chan struct{}, 2)
	firstAcquired := make(chan struct{})
	releaseFirst := make(chan struct{})
	lock := func(_ context.Context, _ *sql.Tx) (func(), error) {
		sequence := count.Add(1)
		if sequence == 1 {
			mutex.Lock()
			close(firstAcquired)
			arrivals <- struct{}{}
			<-releaseFirst
			return mutex.Unlock, nil
		}
		<-firstAcquired
		arrivals <- struct{}{}
		mutex.Lock()
		return mutex.Unlock, nil
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- runContentFeaturedBackfillAt(context.Background(), db, path, lock, writeContentTaxonomyAtomic)
		}()
	}
	<-arrivals
	<-arrivals
	if err := os.WriteFile(path, []byte(`{"featuredArticleIds":[],"contentV1Migration":{"state":"completed","completedAt":"2026-10-26T00:00:00Z"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if got := count.Load(); got != 2 {
		t.Fatalf("lock acquisitions = %d, want 2", got)
	}
}

func TestRunContentFeaturedBackfillRollsBackWhenSourceReadFailsWithoutMarker(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := writeTestTaxonomy(t, `{"featuredArticleIds":[9]}`)
	mock.ExpectBegin()
	expectAdvisoryLock(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, COALESCE(updated_at, NOW())`)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if err := RunContentFeaturedBackfillAt(context.Background(), db, path); err == nil {
		t.Fatal("expected source read failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["contentV1Migration"]; exists {
		t.Fatal("failed source read wrote a marker")
	}
}

func expectTaggedRows(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id", "updated_at"})
	for _, id := range ids {
		rows.AddRow(id, time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC))
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, COALESCE(updated_at, NOW())`)).WillReturnRows(rows)
}

func expectCurrentFeaturedRows(mock sqlmock.Sqlmock, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery(`SELECT id\s+FROM content_articles\s+WHERE featured_at IS NOT NULL`).WillReturnRows(rows)
}

func expectApplyFeaturedAndCommit(mock sqlmock.Sqlmock, id int64) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET featured_at = $1 WHERE id = $2`)).
		WithArgs(sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET featured_at = NULL WHERE featured_at IS NOT NULL AND NOT (id = ANY($1))`)).
		WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET tags = COALESCE(tags, '[]'::jsonb) - '精选' WHERE tags @> '["精选"]'::jsonb`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, featured_at\s+FROM content_articles`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "featured_at"}).AddRow(id, time.Now().UTC()))
	mock.ExpectCommit()
}

func TestRunContentFeaturedBackfillFinalMarkerFailureRecoversOnRerun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := writeTestTaxonomy(t, `{"featuredArticleIds":[]}`)
	writeCount := 0
	writer := func(path string, raw map[string]json.RawMessage) error {
		writeCount++
		if writeCount == 2 {
			return errors.New("simulated final marker write failure")
		}
		return writeContentTaxonomyAtomic(path, raw)
	}

	// First pass writes a durable pending snapshot, commits the DB changes, then
	// deliberately fails while publishing the completion marker.
	mock.ExpectBegin()
	expectAdvisoryLock(mock)
	expectTaggedRows(mock, 9)
	expectCurrentFeaturedRows(mock)
	expectApplyFeaturedAndCommit(mock, 9)
	if err := runContentFeaturedBackfillAt(context.Background(), db, path, acquireContentFeaturedBackfillLock, writer); err == nil {
		t.Fatal("expected completion marker write failure")
	}
	pendingRaw, _, pending, err := readContentTaxonomy(path)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != "pending" || pending.StartedAt == "" || pending.TargetFeaturedIDs == nil || len(pending.TargetFeaturedIDs) != 1 || pending.TargetFeaturedIDs[0] != 9 {
		t.Fatalf("pending snapshot not durable or incomplete: %+v", pending)
	}
	if _, exists := pendingRaw["contentV1Migration"]; !exists {
		t.Fatal("pending marker missing after final write failure")
	}

	// On rerun, the current DB set matches the recorded target: do not replay
	// featured_at writes; reconcile and finalize the exact pending snapshot.
	mock.ExpectBegin()
	expectAdvisoryLock(mock)
	expectTaggedRows(mock)
	expectCurrentFeaturedRows(mock, 9)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET tags = COALESCE(tags, '[]'::jsonb) - '精选' WHERE tags @> '["精选"]'::jsonb`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id, featured_at\s+FROM content_articles`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "featured_at"}).AddRow(int64(9), time.Now().UTC()))
	mock.ExpectCommit()
	if err := runContentFeaturedBackfillAt(context.Background(), db, path, acquireContentFeaturedBackfillLock, writeContentTaxonomyAtomic); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	_, ids, completed, err := readContentTaxonomy(path)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != "completed" || completed.CompletedAt == "" || len(ids) != 0 {
		t.Fatalf("rerun did not finalize durable snapshot: ids=%v marker=%+v", ids, completed)
	}
	if writeCount != 2 {
		t.Fatalf("first pass writer calls = %d, want 2", writeCount)
	}
}

func TestRunContentFeaturedBackfillEqualTargetRecoversAfterMarkerFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := writeTestTaxonomy(t, `{"featuredArticleIds":[]}`)
	writes := 0
	writer := func(path string, raw map[string]json.RawMessage) error {
		writes++
		if writes == 2 {
			return errors.New("simulated final marker write failure")
		}
		return writeContentTaxonomyAtomic(path, raw)
	}

	// Existing featured articles win, so previous and target are both [9]. The
	// transaction still removes a legacy tag before the final marker write fails.
	mock.ExpectBegin()
	expectAdvisoryLock(mock)
	expectTaggedRows(mock, 5)
	expectCurrentFeaturedRows(mock, 9)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET tags = COALESCE(tags, '[]'::jsonb) - '精选' WHERE tags @> '["精选"]'::jsonb`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, featured_at\s+FROM content_articles`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "featured_at"}).AddRow(int64(9), time.Now().UTC()))
	mock.ExpectCommit()
	if err := runContentFeaturedBackfillAt(context.Background(), db, path, acquireContentFeaturedBackfillLock, writer); err == nil {
		t.Fatal("expected completion marker write failure")
	}

	// On rerun the same featured IDs plus cleared tags prove the transaction
	// committed, despite previous == target; do not misclassify as a rollback.
	mock.ExpectBegin()
	expectAdvisoryLock(mock)
	expectTaggedRows(mock)
	expectCurrentFeaturedRows(mock, 9)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE content_articles SET tags = COALESCE(tags, '[]'::jsonb) - '精选' WHERE tags @> '["精选"]'::jsonb`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT id, featured_at\s+FROM content_articles`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "featured_at"}).AddRow(int64(9), time.Now().UTC()))
	mock.ExpectCommit()
	if err := runContentFeaturedBackfillAt(context.Background(), db, path, acquireContentFeaturedBackfillLock, writeContentTaxonomyAtomic); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	_, _, marker, err := readContentTaxonomy(path)
	if err != nil {
		t.Fatal(err)
	}
	if marker.State != "completed" || marker.CompletedAt == "" {
		t.Fatalf("marker not finalized: %+v", marker)
	}
}

func TestContentFeaturedBackfillSourceIDsSortDescending(t *testing.T) {
	got := sourceIDs([]contentFeaturedSourceRow{{ID: 2}, {ID: 9}, {ID: 4}})
	want := []int64{9, 4, 2}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
