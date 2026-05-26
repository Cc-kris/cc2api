package repository

import (
	"context"
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type balanceLowNotifyRepository struct {
	db *sql.DB
}

func NewBalanceLowNotifyRepository(db *sql.DB) service.BalanceLowNotifyRepository {
	return &balanceLowNotifyRepository{db: db}
}

func (r *balanceLowNotifyRepository) ListUsersBelowBalanceThreshold(ctx context.Context, threshold float64, excludedUserIDs []int64) ([]service.BalanceLowNotifyUser, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.username, u.balance::float8
		FROM users u
		LEFT JOIN balance_low_notify_states s ON s.user_id = u.id
		WHERE u.deleted_at IS NULL
		  AND u.status = $3
		  AND u.balance < $1
		  AND TRIM(COALESCE(u.email, '')) <> ''
		  AND s.user_id IS NULL
		  AND (COALESCE(array_length($2::bigint[], 1), 0) = 0 OR NOT (u.id = ANY($2::bigint[])))
		ORDER BY u.id ASC
	`, threshold, pq.Array(excludedUserIDs), service.StatusActive)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	users := make([]service.BalanceLowNotifyUser, 0)
	for rows.Next() {
		var user service.BalanceLowNotifyUser
		if err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.Balance); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *balanceLowNotifyRepository) ResetUsersAtOrAboveBalanceThreshold(ctx context.Context, threshold float64, excludedUserIDs []int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM balance_low_notify_states s
		USING users u
		WHERE u.id = s.user_id
		  AND (
		    u.deleted_at IS NOT NULL
		    OR u.status <> $3
		    OR u.balance >= $1
		    OR (COALESCE(array_length($2::bigint[], 1), 0) > 0 AND u.id = ANY($2::bigint[]))
		  )
	`, threshold, pq.Array(excludedUserIDs), service.StatusActive)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return rows, nil
}

func (r *balanceLowNotifyRepository) MarkBalanceLowNotified(ctx context.Context, userID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO balance_low_notify_states (user_id, notified_at, created_at, updated_at)
		VALUES ($1, NOW(), NOW(), NOW())
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}
