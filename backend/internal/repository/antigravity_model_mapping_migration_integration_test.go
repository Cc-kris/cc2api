//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

// V02-T20/T21 / V02-AC47：迁移 220、221 只给已保存非空 model_mapping 的 Antigravity 账号补缺失键。
// TestMain 已经跑过全部迁移，这里先插入账号，再重放迁移 SQL 断言。

type antigravityMappingMigrationCase struct {
	file string
	keys []string
}

var antigravityMappingMigrations = []antigravityMappingMigrationCase{
	{
		file: "220_add_claude_next_models_to_antigravity_mapping.sql",
		keys: []string{
			"claude-fable-5", "claude-fable-5-1", "claude-opus-4-8",
			"claude-opus-5-5", "claude-opus-5-5-high", "claude-opus-5-5-medium", "claude-opus-5-5-low",
			"claude-sonnet-5-5", "claude-sonnet-5-5-high", "claude-sonnet-5-5-medium", "claude-sonnet-5-5-low",
		},
	},
}

func insertMigrationTestAccount(t *testing.T, ctx context.Context, name, platform string, credentials map[string]any, deleted bool) int64 {
	t.Helper()
	raw, err := json.Marshal(credentials)
	require.NoError(t, err)
	var id int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`INSERT INTO accounts(name, platform, type, credentials, deleted_at)
		 VALUES($1, $2, 'oauth', $3::jsonb, CASE WHEN $4 THEN NOW() ELSE NULL END) RETURNING id`,
		name, platform, string(raw), deleted,
	).Scan(&id))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM accounts WHERE id=$1`, id)
	})
	return id
}

func loadMigrationTestCredentials(t *testing.T, ctx context.Context, id int64) map[string]any {
	t.Helper()
	var raw []byte
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT credentials FROM accounts WHERE id=$1`, id).Scan(&raw))
	var creds map[string]any
	require.NoError(t, json.Unmarshal(raw, &creds))
	return creds
}

func replayMigrationFile(t *testing.T, ctx context.Context, file string) {
	t.Helper()
	sqlText, err := migrations.FS.ReadFile(file)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, string(sqlText))
	require.NoError(t, err)
}

func TestAntigravityModelMappingMigration(t *testing.T) {
	for _, mc := range antigravityMappingMigrations {
		t.Run(mc.file, func(t *testing.T) {
			ctx := context.Background()
			suffix := time.Now().UnixNano()
			name := func(kind string) string { return fmt.Sprintf("ag-mapping-migration-%s-%d", kind, suffix) }

			// 第 1 类：没有 model_mapping
			noMapping := insertMigrationTestAccount(t, ctx, name("none"), "antigravity",
				map[string]any{"access_token": "tok"}, false)
			// 第 2 类：model_mapping 为 {}（不限制模型）
			emptyMapping := insertMigrationTestAccount(t, ctx, name("empty"), "antigravity",
				map[string]any{"access_token": "tok", "model_mapping": map[string]any{}}, false)
			// 第 3 类：有映射，缺新键
			missingKeys := insertMigrationTestAccount(t, ctx, name("missing"), "antigravity",
				map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}}, false)
			// 第 4 类：有映射，管理员已把新键之一指向别处
			customized := map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}
			customKey := mc.keys[2] // 220: claude-opus-4-8
			customized[customKey] = "claude-sonnet-4-6"
			adminOverride := insertMigrationTestAccount(t, ctx, name("override"), "antigravity",
				map[string]any{"model_mapping": customized}, false)
			// 不在范围内的账号
			otherPlatform := insertMigrationTestAccount(t, ctx, name("other"), "anthropic",
				map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}}, false)
			deleted := insertMigrationTestAccount(t, ctx, name("deleted"), "antigravity",
				map[string]any{"model_mapping": map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}}, true)

			// 重放两次验证幂等
			replayMigrationFile(t, ctx, mc.file)
			replayMigrationFile(t, ctx, mc.file)

			creds := loadMigrationTestCredentials(t, ctx, noMapping)
			_, has := creds["model_mapping"]
			require.False(t, has, "account without model_mapping must stay without it")

			creds = loadMigrationTestCredentials(t, ctx, emptyMapping)
			require.Equal(t, map[string]any{}, creds["model_mapping"], "{} must stay {}")

			creds = loadMigrationTestCredentials(t, ctx, missingKeys)
			mapping, ok := creds["model_mapping"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "claude-sonnet-4-6", mapping["claude-sonnet-4-6"], "existing entry preserved")
			for _, key := range mc.keys {
				want, inDefault := domain.DefaultAntigravityModelMapping[key]
				require.True(t, inDefault, "migration key %s must exist in DefaultAntigravityModelMapping", key)
				require.Equal(t, want, mapping[key], "key %s backfilled with default target", key)
			}
			require.Len(t, mapping, len(mc.keys)+1)

			creds = loadMigrationTestCredentials(t, ctx, adminOverride)
			mapping, ok = creds["model_mapping"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "claude-sonnet-4-6", mapping[customKey], "admin-configured target must not be overwritten")
			for _, key := range mc.keys {
				if key == customKey {
					continue
				}
				require.Equal(t, domain.DefaultAntigravityModelMapping[key], mapping[key], "key %s", key)
			}

			for _, id := range []int64{otherPlatform, deleted} {
				creds = loadMigrationTestCredentials(t, ctx, id)
				require.Equal(t, map[string]any{"claude-sonnet-4-6": "claude-sonnet-4-6"}, creds["model_mapping"],
					"non-antigravity or deleted account must be untouched")
			}
		})
	}
}
