package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/systemmodelpriceversion"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type financeCatalogVersionRepository struct {
	client *dbent.Client
}

func NewFinanceCatalogVersionRepository(client *dbent.Client) service.FinanceCatalogVersionRepository {
	return &financeCatalogVersionRepository{client: client}
}

func (r *financeCatalogVersionRepository) SyncSystemPriceVersions(ctx context.Context, checksum string, effectiveFrom time.Time, versions []service.FinanceSystemPriceVersion) (bool, error) {
	checksum = strings.TrimSpace(checksum)
	if checksum == "" {
		return false, fmt.Errorf("catalog checksum is required")
	}
	if len(versions) == 0 {
		return false, fmt.Errorf("system price versions are required")
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	exists, err := tx.SystemModelPriceVersion.Query().
		Where(systemmodelpriceversion.CatalogChecksumEQ(checksum)).
		Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check catalog version: %w", err)
	}
	if exists {
		return false, nil
	}
	latest, latestErr := tx.SystemModelPriceVersion.Query().
		Where(systemmodelpriceversion.EffectiveToIsNil()).
		Order(dbent.Desc(systemmodelpriceversion.FieldEffectiveFrom), dbent.Desc(systemmodelpriceversion.FieldID)).
		ForUpdate().
		First(ctx)
	if latestErr != nil && !dbent.IsNotFound(latestErr) {
		return false, fmt.Errorf("lock current catalog version: %w", latestErr)
	}
	if latest != nil && !effectiveFrom.After(latest.EffectiveFrom) {
		return false, fmt.Errorf("catalog effective_from must be later than the current active version")
	}
	if _, err = tx.SystemModelPriceVersion.Update().
		Where(systemmodelpriceversion.EffectiveToIsNil()).
		SetEffectiveTo(effectiveFrom).
		Save(ctx); err != nil {
		return false, fmt.Errorf("close previous catalog version: %w", err)
	}
	builders := make([]*dbent.SystemModelPriceVersionCreate, 0, len(versions))
	for _, version := range versions {
		builders = append(builders, tx.SystemModelPriceVersion.Create().
			SetCatalogChecksum(checksum).
			SetProvider(version.Provider).
			SetModelName(version.ModelName).
			SetBillingMode(version.BillingMode).
			SetPriceDetail(version.PriceDetail).
			SetEffectiveFrom(effectiveFrom))
	}
	if _, err = tx.SystemModelPriceVersion.CreateBulk(builders...).Save(ctx); err != nil {
		return false, fmt.Errorf("create system price versions: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
