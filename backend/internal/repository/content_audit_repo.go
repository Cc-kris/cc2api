package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentAuditRepository struct{ client *dbent.Client }

func NewContentAuditRepository(client *dbent.Client) service.ContentAuditLogger {
	return &contentAuditRepository{client: client}
}

func (r *contentAuditRepository) Log(ctx context.Context, e service.ContentAuditEntry) error {
	if r == nil || r.client == nil {
		return nil
	}
	b := r.client.ContentAuditLog.Create().
		SetContentType(e.ContentType).
		SetAction(e.Action).
		SetBeforeSnapshot(nonNilSnapshot(e.Before)).
		SetAfterSnapshot(nonNilSnapshot(e.After))
	if e.ContentID != nil {
		b.SetContentID(*e.ContentID)
	}
	if e.ActorID != nil {
		b.SetActorID(*e.ActorID)
	}
	if e.RequestID != "" {
		b.SetRequestID(e.RequestID)
	}
	_, err := b.Save(ctx)
	return err
}

func nonNilSnapshot(v map[string]any) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(v))
	for k, value := range v {
		out[k] = value
	}
	return out
}

var _ service.ContentAuditLogger = (*contentAuditRepository)(nil)
