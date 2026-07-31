package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

type FinanceRuntime struct {
	catalogSync *FinanceCatalogVersionSync
	scanner     *FinanceUsageScanner
	alerts      *FinanceAlertService
	revenue     *FinanceRevenueRecognitionService
	backfill    *FinanceBackfillService
	exporter    *FinanceExportService
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func ProvideFinanceRuntime(catalogSync *FinanceCatalogVersionSync, scanner *FinanceUsageScanner, alerts *FinanceAlertService, revenue *FinanceRevenueRecognitionService, backfill *FinanceBackfillService, exporter *FinanceExportService) *FinanceRuntime {
	if backfill != nil {
		backfill.revenue = revenue
	}
	runtime := &FinanceRuntime{catalogSync: catalogSync, scanner: scanner, alerts: alerts, revenue: revenue, backfill: backfill, exporter: exporter}
	runtime.Start()
	return runtime
}

func (r *FinanceRuntime) Start() {
	if r == nil || r.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.run(ctx)
	}()
}

func (r *FinanceRuntime) Stop() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
	r.cancel = nil
}

func (r *FinanceRuntime) run(ctx context.Context) {
	catalogTicker := time.NewTicker(time.Hour)
	scannerTicker := time.NewTicker(30 * time.Second)
	alertTicker := time.NewTicker(time.Minute)
	revenueTicker := time.NewTicker(time.Hour)
	backfillTicker := time.NewTicker(2 * time.Second)
	exportTicker := time.NewTicker(2 * time.Second)
	defer catalogTicker.Stop()
	defer scannerTicker.Stop()
	defer alertTicker.Stop()
	defer revenueTicker.Stop()
	defer backfillTicker.Stop()
	defer exportTicker.Stop()
	r.syncCatalog(ctx)
	r.scan(ctx)
	r.scanAlerts(ctx)
	r.recognizeRevenue(ctx)
	r.runBackfill(ctx)
	r.runExport(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-catalogTicker.C:
			r.syncCatalog(ctx)
		case <-scannerTicker.C:
			r.scan(ctx)
		case <-alertTicker.C:
			r.scanAlerts(ctx)
		case <-revenueTicker.C:
			r.recognizeRevenue(ctx)
		case <-backfillTicker.C:
			r.runBackfill(ctx)
		case <-exportTicker.C:
			r.runExport(ctx)
		}
	}
}

func (r *FinanceRuntime) runExport(ctx context.Context) {
	if r.exporter == nil {
		return
	}
	if err := r.exporter.RunNext(ctx); err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] export job failed: %v", err)
	}
}

func (r *FinanceRuntime) runBackfill(ctx context.Context) {
	if r.backfill == nil {
		return
	}
	if err := r.backfill.RunNextBatch(ctx); err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] backfill batch failed: %v", err)
	}
}

func (r *FinanceRuntime) recognizeRevenue(ctx context.Context) {
	if r.revenue == nil {
		return
	}
	now := time.Now()
	timezone := r.revenue.Timezone()
	if _, err := r.revenue.BackfillUnrecognized(ctx, now, timezone, 366); err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] historical subscription revenue backfill failed: error=%v", err)
	}
	for _, date := range []time.Time{now.AddDate(0, 0, -1), now} {
		if _, err := r.revenue.RecognizeDate(ctx, date, timezone); err != nil && ctx.Err() == nil {
			logger.LegacyPrintf("service.finance", "[Finance] subscription revenue recognition failed: date=%s error=%v", date.Format("2006-01-02"), err)
		}
	}
}

func (r *FinanceRuntime) scanAlerts(ctx context.Context) {
	if _, err := r.alerts.Scan(ctx); err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] alert scan failed: %v", err)
	}
}

func (r *FinanceRuntime) syncCatalog(ctx context.Context) {
	if _, err := r.catalogSync.Sync(ctx); err != nil && ctx.Err() == nil {
		logger.LegacyPrintf("service.finance", "[Finance] catalog version sync failed: %v", err)
	}
}

func (r *FinanceRuntime) scan(ctx context.Context) {
	result, err := r.scanner.RunBatch(ctx)
	if err != nil {
		if ctx.Err() == nil {
			logger.LegacyPrintf("service.finance", "[Finance] usage scan failed: %v", err)
		}
		return
	}
	if result.Failed > 0 {
		logger.LegacyPrintf("service.finance", "[Finance] usage scan completed with failures: processed=%d succeeded=%d failed=%d errors=%v", result.Processed, result.Succeeded, result.Failed, result.Errors)
	}
	if r.revenue != nil && len(result.SucceededAt) > 0 {
		timezone := r.revenue.Timezone()
		location, locationErr := time.LoadLocation(timezone)
		if locationErr != nil {
			logger.LegacyPrintf("service.finance", "[Finance] subscription revenue reallocation timezone is invalid: timezone=%s error=%v", timezone, locationErr)
			return
		}
		datesByKey := make(map[string]time.Time, len(result.SucceededAt))
		for _, occurredAt := range result.SucceededAt {
			date := financeRecognitionDate(occurredAt, location)
			datesByKey[date.Format("2006-01-02")] = date
		}
		keys := make([]string, 0, len(datesByKey))
		for key := range datesByKey {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, err = r.revenue.RecognizeDate(ctx, datesByKey[key], timezone); err != nil && ctx.Err() == nil {
				logger.LegacyPrintf("service.finance", "[Finance] subscription revenue reallocation after usage scan failed: date=%s error=%v", key, err)
				return
			}
		}
	}
}
