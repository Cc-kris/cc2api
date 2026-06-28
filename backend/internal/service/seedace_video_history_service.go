package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const maxSeedaceVideoHistoryRecords = 20
const maxSeedaceVideoHistoryMessages = 200

var ErrSeedaceVideoHistoryUnavailable = infraerrors.ServiceUnavailable("SEEDACE_VIDEO_HISTORY_UNAVAILABLE", "video generation history service is unavailable")

type SeedaceVideoHistoryService struct {
	entClient *dbent.Client
}

type SeedaceVideoHistoryMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
	Status  string `json:"status,omitempty"`
	TaskID  string `json:"taskId,omitempty"`
	Error   string `json:"error,omitempty"`
}

type SeedaceVideoHistoryRecord struct {
	SessionID       string                       `json:"id"`
	Summary         string                       `json:"summary"`
	GenerationCount int                          `json:"generationCount"`
	UpdatedAt       time.Time                    `json:"updatedAt"`
	Messages        []SeedaceVideoHistoryMessage `json:"messages"`
}

type UpsertSeedaceVideoHistoryInput struct {
	UserID          int64
	SessionID       string
	Summary         string
	GenerationCount int
	Messages        []SeedaceVideoHistoryMessage
}

func NewSeedaceVideoHistoryService(entClient *dbent.Client) *SeedaceVideoHistoryService {
	return &SeedaceVideoHistoryService{entClient: entClient}
}

func (s *SeedaceVideoHistoryService) List(ctx context.Context, userID int64, limit int) ([]SeedaceVideoHistoryRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, ErrSeedaceVideoHistoryUnavailable
	}
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "invalid user id")
	}
	if limit <= 0 || limit > maxSeedaceVideoHistoryRecords {
		limit = maxSeedaceVideoHistoryRecords
	}
	db := sqlDBFromEnt(s.entClient)
	if db == nil {
		return nil, ErrSeedaceVideoHistoryUnavailable
	}

	rows, err := db.QueryContext(ctx, `
		SELECT session_id, summary, generation_count, updated_at, messages
		FROM seedace_video_session_histories
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list seedace video history: %w", err)
	}
	defer rows.Close()

	records := make([]SeedaceVideoHistoryRecord, 0)
	for rows.Next() {
		var record SeedaceVideoHistoryRecord
		var rawMessages []byte
		if err := rows.Scan(&record.SessionID, &record.Summary, &record.GenerationCount, &record.UpdatedAt, &rawMessages); err != nil {
			return nil, fmt.Errorf("scan seedace video history: %w", err)
		}
		if err := json.Unmarshal(rawMessages, &record.Messages); err != nil {
			record.Messages = []SeedaceVideoHistoryMessage{}
		}
		record.Messages = normalizeSeedaceVideoHistoryMessages(record.Messages)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seedace video history: %w", err)
	}
	return records, nil
}

func (s *SeedaceVideoHistoryService) Upsert(ctx context.Context, input UpsertSeedaceVideoHistoryInput) (*SeedaceVideoHistoryRecord, error) {
	if s == nil || s.entClient == nil {
		return nil, ErrSeedaceVideoHistoryUnavailable
	}
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Summary = strings.TrimSpace(input.Summary)
	if input.UserID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER_ID", "invalid user id")
	}
	if input.SessionID == "" {
		return nil, infraerrors.BadRequest("INVALID_SESSION_ID", "invalid session id")
	}
	if input.Summary == "" {
		input.Summary = "未命名会话"
	}
	if len([]rune(input.Summary)) > 20 {
		input.Summary = string([]rune(input.Summary)[:20])
	}
	if input.GenerationCount <= 0 {
		return nil, infraerrors.BadRequest("INVALID_GENERATION_COUNT", "invalid generation count")
	}
	messages := normalizeSeedaceVideoHistoryMessages(input.Messages)
	if len(messages) == 0 {
		return nil, infraerrors.BadRequest("INVALID_MESSAGES", "messages cannot be empty")
	}
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal seedace video history messages: %w", err)
	}
	db := sqlDBFromEnt(s.entClient)
	if db == nil {
		return nil, ErrSeedaceVideoHistoryUnavailable
	}

	var record SeedaceVideoHistoryRecord
	var rawMessages []byte
	err = db.QueryRowContext(ctx, `
		INSERT INTO seedace_video_session_histories (user_id, session_id, summary, generation_count, messages, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, NOW())
		ON CONFLICT (user_id, session_id)
		DO UPDATE SET
			summary = EXCLUDED.summary,
			generation_count = EXCLUDED.generation_count,
			messages = EXCLUDED.messages,
			updated_at = NOW()
		RETURNING session_id, summary, generation_count, updated_at, messages
	`, input.UserID, input.SessionID, input.Summary, input.GenerationCount, string(messagesJSON)).Scan(&record.SessionID, &record.Summary, &record.GenerationCount, &record.UpdatedAt, &rawMessages)
	if err != nil {
		return nil, fmt.Errorf("upsert seedace video history: %w", err)
	}
	if err := json.Unmarshal(rawMessages, &record.Messages); err != nil {
		record.Messages = messages
	}
	record.Messages = normalizeSeedaceVideoHistoryMessages(record.Messages)
	return &record, nil
}

func normalizeSeedaceVideoHistoryMessages(messages []SeedaceVideoHistoryMessage) []SeedaceVideoHistoryMessage {
	out := make([]SeedaceVideoHistoryMessage, 0, len(messages))
	for _, message := range messages {
		message.ID = strings.TrimSpace(message.ID)
		message.Role = strings.TrimSpace(message.Role)
		message.Content = strings.TrimSpace(message.Content)
		message.Status = strings.TrimSpace(message.Status)
		message.TaskID = strings.TrimSpace(message.TaskID)
		message.Error = strings.TrimSpace(message.Error)
		if message.ID == "" || message.Content == "" || (message.Role != "user" && message.Role != "assistant") {
			continue
		}
		if message.Status != "generating" && message.Status != "completed" && message.Status != "failed" {
			message.Status = ""
		}
		out = append(out, message)
		if len(out) >= maxSeedaceVideoHistoryMessages {
			break
		}
	}
	return out
}

func sqlDBFromEnt(client *dbent.Client) *sql.DB {
	if client == nil {
		return nil
	}
	driver := client.Driver()
	type dbProvider interface{ DB() *sql.DB }
	provider, ok := driver.(dbProvider)
	if !ok || provider.DB() == nil {
		return nil
	}
	return provider.DB()
}
