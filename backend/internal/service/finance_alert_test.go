package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type financeAlertRepoStub struct {
	signals     []FinanceAlertSignal
	upserted    []FinanceAlertSignal
	listRequest FinanceAlertListRequest
	updatedID   int64
	updatedTo   string
	updatedNote string
	actorID     int64
}

func (s *financeAlertRepoStub) CollectFinanceAlertSignals(context.Context, time.Time) ([]FinanceAlertSignal, error) {
	return s.signals, nil
}
func (s *financeAlertRepoStub) UpsertFinanceAlertSignals(_ context.Context, signals []FinanceAlertSignal) error {
	s.upserted = signals
	return nil
}
func (s *financeAlertRepoStub) ListFinanceAlerts(_ context.Context, _ FinanceReportFilter, request FinanceAlertListRequest) ([]FinanceAlert, int64, error) {
	s.listRequest = request
	return []FinanceAlert{{ID: 1}}, 1, nil
}
func (s *financeAlertRepoStub) UpdateFinanceAlertStatus(_ context.Context, id int64, status, note string, actorID int64, _ time.Time) (*FinanceAlert, error) {
	s.updatedID, s.updatedTo, s.updatedNote, s.actorID = id, status, note, actorID
	return &FinanceAlert{ID: id, Status: status}, nil
}

func TestFinanceAlertServiceScanUpsertsAllSignals(t *testing.T) {
	repo := &financeAlertRepoStub{signals: []FinanceAlertSignal{{AlertType: "missing_price"}, {AlertType: "wallet_low_balance"}}}
	count, err := NewFinanceAlertService(repo).Scan(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, repo.signals, repo.upserted)
}

func TestFinanceAlertServiceListValidatesAndNormalizes(t *testing.T) {
	repo := &financeAlertRepoStub{}
	items, total, err := NewFinanceAlertService(repo).List(context.Background(), FinanceReportFilter{}, FinanceAlertListRequest{
		AlertType: "missing_price", Severity: "warning", Status: "open", Page: 0, PageSize: 0,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(1), total)
	require.Equal(t, 1, repo.listRequest.Page)
	require.Equal(t, 20, repo.listRequest.PageSize)

	_, _, err = NewFinanceAlertService(repo).List(context.Background(), FinanceReportFilter{}, FinanceAlertListRequest{AlertType: "unknown"})
	require.EqualError(t, err, "alert_type is invalid")
}

func TestFinanceAlertServiceUpdateStatusRequiresNote(t *testing.T) {
	repo := &financeAlertRepoStub{}
	svc := NewFinanceAlertService(repo)
	_, err := svc.UpdateStatus(context.Background(), 7, FinanceAlertStatusUpdate{Status: "resolved"}, 9)
	require.EqualError(t, err, "note is required")

	item, err := svc.UpdateStatus(context.Background(), 7, FinanceAlertStatusUpdate{Status: "acknowledged", Note: "已核对"}, 9)
	require.NoError(t, err)
	require.Equal(t, int64(7), item.ID)
	require.Equal(t, "acknowledged", repo.updatedTo)
	require.Equal(t, int64(9), repo.actorID)
}
