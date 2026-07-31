//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountFinanceCounterOwnerIsGlobalAcrossAccounts(t *testing.T) {
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var firstAccountID, secondAccountID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials) VALUES($1,'openai','api_key','{}') RETURNING id`, fmt.Sprintf("counter-owner-a-%d", suffix)).Scan(&firstAccountID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO accounts(name,platform,type,credentials) VALUES($1,'openai','api_key','{}') RETURNING id`, fmt.Sprintf("counter-owner-b-%d", suffix)).Scan(&secondAccountID))
	var upstreamID, walletID, secondWalletID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstreams(name,base_url,normalized_base_url) VALUES($1,$2,$2) RETURNING id`, fmt.Sprintf("counter-owner-upstream-%d", suffix), fmt.Sprintf("https://counter-owner-%d.example", suffix)).Scan(&upstreamID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstream_wallets(upstream_id,name,base_url,pricing_adapter,balance_adapter,quota_adapter,currency,balance_scope_key,enabled) VALUES($1,$2,$3,'manual','manual','none','USD','shared-main',true) RETURNING id`, upstreamID, fmt.Sprintf("wallet-%d", suffix), fmt.Sprintf("https://counter-owner-%d.example", suffix)).Scan(&walletID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstream_wallets(upstream_id,name,base_url,pricing_adapter,balance_adapter,quota_adapter,currency,balance_scope_key,enabled) VALUES($1,$2,$3,'manual','manual','none','USD','shared-main',true) RETURNING id`, upstreamID, fmt.Sprintf("wallet-2-%d", suffix), fmt.Sprintf("https://counter-owner-%d.example", suffix)).Scan(&secondWalletID))
	var protocolID, firstVersionID, secondVersionID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocols(code,name,protocol_type,status) VALUES($1,$2,'http_json','published') RETURNING id`, fmt.Sprintf("counter_owner_%d", suffix), "counter owner protocol").Scan(&protocolID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocol_versions(protocol_id,version,config,checksum,validation_status) VALUES($1,1,'{}','first','valid') RETURNING id`, protocolID).Scan(&firstVersionID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocol_versions(protocol_id,version,config,checksum,validation_status) VALUES($1,2,'{}','second','valid') RETURNING id`, protocolID).Scan(&secondVersionID))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_finance_counter_owners WHERE wallet_id IN ($1,$2)`, walletID, secondWalletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_wallets WHERE id IN ($1,$2)`, walletID, secondWalletID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_finance_protocol_versions WHERE protocol_id=$1`, protocolID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstream_finance_protocols WHERE id=$1`, protocolID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM upstreams WHERE id=$1`, upstreamID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id IN ($1,$2)`, firstAccountID, secondAccountID)
	})

	repo := NewAccountFinanceSnapshotRepository(integrationDB)
	require.NoError(t, repo.ClaimCounterOwner(ctx, "global-counter-identity", walletID, &firstVersionID, firstAccountID, nil, nil))
	require.NoError(t, repo.ClaimCounterOwner(ctx, "global-counter-identity", secondWalletID, &secondVersionID, firstAccountID, nil, nil))
	err := repo.ClaimCounterOwner(ctx, "global-counter-identity", secondWalletID, &secondVersionID, secondAccountID, nil, nil)
	require.True(t, errors.Is(err, service.ErrAccountFinanceCounterOwnerConflict), "err=%v", err)
}
