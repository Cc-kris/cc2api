package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func transientDatabaseError(code string) error {
	return &pq.Error{Code: pq.ErrorCode(code), Message: "database is temporarily unavailable"}
}

func TestIsTransientDatabaseInitializationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "postgres is starting", err: transientDatabaseError("57P03"), want: true},
		{name: "connection failure", err: fmt.Errorf("wrapped: %w", transientDatabaseError("08006")), want: true},
		{name: "authentication failure", err: transientDatabaseError("28P01"), want: false},
		{name: "migration error", err: errors.New("migration checksum mismatch"), want: false},
		// CCAI 本地补充：应用先于数据库启动时拨号被拒。
		{name: "connection refused", err: fmt.Errorf("acquire migrations lock: %w", &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}), want: true},
		{name: "dns failure", err: fmt.Errorf("acquire migrations lock: %w", &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "postgres", IsNotFound: true}}), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isTransientDatabaseInitializationError(tt.err))
		})
	}
}

func TestInitializeDatabaseWithRetryEventuallySucceeds(t *testing.T) {
	attempts := 0
	var delays []time.Duration
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		if attempts <= 3 {
			return transientDatabaseError("57P03")
		}
		return nil
	}, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 4, attempts)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}, delays)
}

func TestInitializeDatabaseWithRetryFailsFastForPermanentError(t *testing.T) {
	attempts := 0
	waitCalled := false
	permanentErr := transientDatabaseError("28P01")
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		return permanentErr
	}, func(_ context.Context, _ time.Duration) error {
		waitCalled = true
		return nil
	})

	require.ErrorIs(t, err, permanentErr)
	require.Equal(t, 1, attempts)
	require.False(t, waitCalled)
}

func TestInitializeDatabaseWithRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	err := initializeDatabaseWithRetryWithWait(ctx, func(context.Context) error {
		attempts++
		return transientDatabaseError("57P03")
	}, func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}

func TestInitializeDatabaseWithRetryReturnsLastErrorAfterLimit(t *testing.T) {
	attempts := 0
	lastErr := transientDatabaseError("08006")
	var delays []time.Duration
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		return lastErr
	}, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	})

	require.ErrorIs(t, err, lastErr)
	require.Equal(t, maxDatabaseInitializationRetries+1, attempts)
	require.Len(t, delays, maxDatabaseInitializationRetries)
	require.Equal(t, databaseInitializationRetryMax, delays[len(delays)-1])
}

func TestInitializeDatabaseWithRetryAllowsIdempotentMigrationRetry(t *testing.T) {
	applied := make(map[string]bool)
	attempts := 0
	err := initializeDatabaseWithRetryWithWait(context.Background(), func(context.Context) error {
		attempts++
		if !applied["001_init.sql"] {
			applied["001_init.sql"] = true
			return transientDatabaseError("57P03")
		}
		return nil
	}, func(_ context.Context, _ time.Duration) error { return nil })

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.True(t, applied["001_init.sql"])
}

// 真实拨号一个没有监听的端口，确认 lib/pq 返回的错误能被识别为可重试。
func TestIsTransientDatabaseInitializationErrorRealConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok, "listener address should be *net.TCPAddr")
	require.NoError(t, ln.Close())

	db, err := sql.Open("postgres", fmt.Sprintf("host=127.0.0.1 port=%d user=x dbname=x sslmode=disable connect_timeout=2", addr.Port))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	pingErr := db.PingContext(context.Background())
	require.Error(t, pingErr)
	require.True(t, isTransientDatabaseInitializationError(fmt.Errorf("acquire migrations lock: %w", pingErr)), "err=%v", pingErr)
}
