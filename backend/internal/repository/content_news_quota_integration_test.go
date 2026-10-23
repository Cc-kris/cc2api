//go:build integration

package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContentNewsQuotaReservationIsAtomicAndReleasable(t *testing.T) {
	ctx := context.Background()
	day := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	_, err := integrationDB.ExecContext(ctx, `DELETE FROM content_news_quota_reservations WHERE quota_day=$1::date AND quota_kind='fetch'`, day)
	require.NoError(t, err)
	var reserved int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO content_news_quota_reservations (quota_day,quota_kind,reserved_count) VALUES ($1::date,'fetch',1) RETURNING reserved_count`, day).Scan(&reserved))
	require.Equal(t, 1, reserved)
	require.NoError(t, integrationDB.QueryRowContext(ctx, `UPDATE content_news_quota_reservations SET reserved_count=reserved_count+1 WHERE quota_day=$1::date AND quota_kind='fetch' AND reserved_count < 2 RETURNING reserved_count`, day).Scan(&reserved))
	require.Equal(t, 2, reserved)
	err = integrationDB.QueryRowContext(ctx, `UPDATE content_news_quota_reservations SET reserved_count=reserved_count+1 WHERE quota_day=$1::date AND quota_kind='fetch' AND reserved_count < 2 RETURNING reserved_count`, day).Scan(&reserved)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = integrationDB.ExecContext(ctx, `UPDATE content_news_quota_reservations SET reserved_count=GREATEST(reserved_count-1,0) WHERE quota_day=$1::date AND quota_kind='fetch'`, day)
	require.NoError(t, err)
}

func TestContentNewsQuotaReservationUsesSiteCalendarDay(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	day := time.Date(2026, 9, 11, 0, 0, 0, 0, loc)
	var stored string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `SELECT $1::date::text`, day.Format("2006-01-02")).Scan(&stored))
	require.Equal(t, "2026-09-11", stored)
}
