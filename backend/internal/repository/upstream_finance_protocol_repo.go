package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type upstreamFinanceProtocolRepository struct{ db *sql.DB }

func NewUpstreamFinanceProtocolRepository(db *sql.DB) service.UpstreamFinanceProtocolRepository {
	return &upstreamFinanceProtocolRepository{db: db}
}

func (r *upstreamFinanceProtocolRepository) ListProtocols(ctx context.Context, filter service.FinanceProtocolListFilter) ([]service.UpstreamFinanceProtocol, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("p.status=$%d", len(args)))
	}
	if filter.ProtocolType != "" {
		args = append(args, filter.ProtocolType)
		where = append(where, fmt.Sprintf("p.protocol_type=$%d", len(args)))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM upstream_finance_protocols p WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count upstream finance protocols: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := protocolSelect + " WHERE " + whereSQL + fmt.Sprintf(" ORDER BY p.updated_at DESC,p.id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list upstream finance protocols: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFinanceProtocol, 0)
	for rows.Next() {
		item, scanErr := scanFinanceProtocol(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

const protocolSelect = `
SELECT p.id,p.code,p.name,p.protocol_type,p.status,p.current_version_id,p.created_by,p.updated_by,p.created_at,p.updated_at,
       v.id,v.protocol_id,v.version,v.config,v.checksum,v.validation_status,v.validation_result,v.published_at,v.created_by,v.created_at
FROM upstream_finance_protocols p
LEFT JOIN upstream_finance_protocol_versions v ON v.id=p.current_version_id`

func (r *upstreamFinanceProtocolRepository) GetProtocol(ctx context.Context, id int64) (*service.UpstreamFinanceProtocol, error) {
	item, err := scanFinanceProtocol(r.db.QueryRowContext(ctx, protocolSelect+" WHERE p.id=$1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamFinanceProtocolNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get upstream finance protocol: %w", err)
	}
	if item.CurrentVersion == nil {
		versions, listErr := r.ListVersions(ctx, id)
		if listErr != nil {
			return nil, listErr
		}
		if len(versions) > 0 {
			item.CurrentVersion = &versions[0]
		}
	}
	return item, nil
}

func (r *upstreamFinanceProtocolRepository) GetVersion(ctx context.Context, id int64) (*service.UpstreamFinanceProtocolVersion, error) {
	version, err := scanFinanceProtocolVersion(r.db.QueryRowContext(ctx, protocolVersionSelect+" WHERE id=$1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamFinanceProtocolNotFound
	}
	return version, err
}

const protocolVersionSelect = `SELECT id,protocol_id,version,config,checksum,validation_status,validation_result,published_at,created_by,created_at FROM upstream_finance_protocol_versions`

func (r *upstreamFinanceProtocolRepository) ListVersions(ctx context.Context, protocolID int64) ([]service.UpstreamFinanceProtocolVersion, error) {
	rows, err := r.db.QueryContext(ctx, protocolVersionSelect+" WHERE protocol_id=$1 ORDER BY version DESC", protocolID)
	if err != nil {
		return nil, fmt.Errorf("list protocol versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UpstreamFinanceProtocolVersion, 0)
	for rows.Next() {
		version, scanErr := scanFinanceProtocolVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *version)
	}
	return items, rows.Err()
}

func (r *upstreamFinanceProtocolRepository) CreateProtocol(ctx context.Context, input service.FinanceProtocolCreateInput, validation service.FinanceProtocolValidationResult) (*service.UpstreamFinanceProtocol, error) {
	if err := service.ValidateFinanceProtocolCode(input.Code); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var protocol service.UpstreamFinanceProtocol
	err = tx.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocols(code,name,protocol_type,status,created_by,updated_by) VALUES($1,$2,$3,'draft',$4,$4) RETURNING id,created_at,updated_at`, input.Code, input.Name, input.ProtocolType, input.OperatorID).Scan(&protocol.ID, &protocol.CreatedAt, &protocol.UpdatedAt)
	if err != nil {
		if isFinanceProtocolUniqueViolation(err) {
			return nil, service.ErrUpstreamFinanceProtocolConflict
		}
		return nil, fmt.Errorf("create protocol: %w", err)
	}
	configJSON, _ := json.Marshal(input.Config)
	validationJSON, _ := json.Marshal(validation)
	var version service.UpstreamFinanceProtocolVersion
	err = tx.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocol_versions(protocol_id,version,config,checksum,validation_status,validation_result,created_by) VALUES($1,1,$2,$3,'valid',$4,$5) RETURNING id,created_at`, protocol.ID, configJSON, validation.Checksum, validationJSON, input.OperatorID).Scan(&version.ID, &version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create protocol version: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	protocol.Code = input.Code
	protocol.Name = input.Name
	protocol.ProtocolType = input.ProtocolType
	protocol.Status = service.FinanceProtocolStatusDraft
	protocol.CreatedBy = input.OperatorID
	protocol.UpdatedBy = input.OperatorID
	version.ProtocolID = protocol.ID
	version.Version = 1
	version.Config = input.Config
	version.Checksum = validation.Checksum
	version.ValidationStatus = "valid"
	version.ValidationResult = validation
	version.CreatedBy = input.OperatorID
	protocol.CurrentVersion = &version
	return &protocol, nil
}

func (r *upstreamFinanceProtocolRepository) CreateDraftVersion(ctx context.Context, protocolID int64, input service.FinanceProtocolDraftInput, validation service.FinanceProtocolValidationResult) (*service.UpstreamFinanceProtocolVersion, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var lockedID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM upstream_finance_protocols WHERE id=$1 FOR UPDATE`, protocolID).Scan(&lockedID); errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUpstreamFinanceProtocolNotFound
	} else if err != nil {
		return nil, err
	}
	var next int
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM upstream_finance_protocol_versions WHERE protocol_id=$1`, protocolID).Scan(&next); err != nil {
		return nil, err
	}
	configJSON, _ := json.Marshal(input.Config)
	validationJSON, _ := json.Marshal(validation)
	version := &service.UpstreamFinanceProtocolVersion{}
	err = tx.QueryRowContext(ctx, `INSERT INTO upstream_finance_protocol_versions(protocol_id,version,config,checksum,validation_status,validation_result,created_by) VALUES($1,$2,$3,$4,'valid',$5,$6) RETURNING id,created_at`, protocolID, next, configJSON, validation.Checksum, validationJSON, input.OperatorID).Scan(&version.ID, &version.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create draft version: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE upstream_finance_protocols SET name=COALESCE(NULLIF($2,''),name),updated_by=$3,updated_at=NOW() WHERE id=$1`, protocolID, input.Name, input.OperatorID)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, service.ErrUpstreamFinanceProtocolNotFound
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	version.ProtocolID = protocolID
	version.Version = next
	version.Config = input.Config
	version.Checksum = validation.Checksum
	version.ValidationStatus = "valid"
	version.ValidationResult = validation
	version.CreatedBy = input.OperatorID
	return version, nil
}

func (r *upstreamFinanceProtocolRepository) PublishVersion(ctx context.Context, protocolID, versionID int64, operatorID *int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	nowQuery := `UPDATE upstream_finance_protocol_versions SET published_at=COALESCE(published_at,NOW()) WHERE id=$1 AND protocol_id=$2 AND validation_status='valid'`
	result, err := tx.ExecContext(ctx, nowQuery, versionID, protocolID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrUpstreamFinanceProtocolInvalidState
	}
	result, err = tx.ExecContext(ctx, `UPDATE upstream_finance_protocols SET current_version_id=$2,status='published',updated_by=$3,updated_at=NOW() WHERE id=$1`, protocolID, versionID, operatorID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrUpstreamFinanceProtocolNotFound
	}
	return tx.Commit()
}
func (r *upstreamFinanceProtocolRepository) DisableProtocol(ctx context.Context, id int64, operatorID *int64) error {
	result, err := r.db.ExecContext(ctx, `UPDATE upstream_finance_protocols SET status='disabled',updated_by=$2,updated_at=NOW() WHERE id=$1 AND status='published'`, id, operatorID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrUpstreamFinanceProtocolInvalidState
	}
	return nil
}
func (r *upstreamFinanceProtocolRepository) DeleteDraft(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM upstream_finance_protocols WHERE id=$1 AND status='draft' AND current_version_id IS NULL`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return service.ErrUpstreamFinanceProtocolInvalidState
	}
	return nil
}

func (r *upstreamFinanceProtocolRepository) CreateDetectionAudit(ctx context.Context, audit service.FinanceProtocolDetectionAudit) error {
	candidates, err := json.Marshal(audit.Candidates)
	if err != nil {
		return fmt.Errorf("marshal finance protocol detection candidates: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO upstream_finance_protocol_detection_audits(
 account_id,protocol_id,protocol_version_id,status,reason,platform,account_type,base_url_hash,candidates,operator_id
) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		audit.AccountID, audit.ProtocolID, audit.ProtocolVersionID, audit.Status, audit.Reason,
		audit.Platform, audit.AccountType, audit.BaseURLHash, candidates, audit.OperatorID)
	if err != nil {
		return fmt.Errorf("create upstream finance protocol detection audit: %w", err)
	}
	return nil
}

type financeProtocolScanner interface{ Scan(...any) error }

func scanFinanceProtocol(scanner financeProtocolScanner) (*service.UpstreamFinanceProtocol, error) {
	var p service.UpstreamFinanceProtocol
	var currentVersionID, createdBy, updatedBy sql.NullInt64
	var versionID, versionProtocolID sql.NullInt64
	var version sql.NullInt32
	var configJSON, validationJSON []byte
	var checksum, validationStatus sql.NullString
	var publishedAt sql.NullTime
	var versionCreatedBy sql.NullInt64
	var versionCreatedAt sql.NullTime
	err := scanner.Scan(&p.ID, &p.Code, &p.Name, &p.ProtocolType, &p.Status, &currentVersionID, &createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt, &versionID, &versionProtocolID, &version, &configJSON, &checksum, &validationStatus, &validationJSON, &publishedAt, &versionCreatedBy, &versionCreatedAt)
	if err != nil {
		return nil, err
	}
	p.CurrentVersionID = nullInt64Pointer(currentVersionID)
	p.CreatedBy = nullInt64Pointer(createdBy)
	p.UpdatedBy = nullInt64Pointer(updatedBy)
	if versionID.Valid {
		v, err := decodeFinanceProtocolVersion(versionID.Int64, versionProtocolID.Int64, int(version.Int32), configJSON, checksum.String, validationStatus.String, validationJSON, publishedAt, versionCreatedBy, versionCreatedAt.Time)
		if err != nil {
			return nil, err
		}
		p.CurrentVersion = v
	}
	return &p, nil
}
func scanFinanceProtocolVersion(scanner financeProtocolScanner) (*service.UpstreamFinanceProtocolVersion, error) {
	var id, protocolID int64
	var version int
	var configJSON, validationJSON []byte
	var checksum, status string
	var publishedAt sql.NullTime
	var createdBy sql.NullInt64
	var createdAt sql.NullTime
	if err := scanner.Scan(&id, &protocolID, &version, &configJSON, &checksum, &status, &validationJSON, &publishedAt, &createdBy, &createdAt); err != nil {
		return nil, err
	}
	return decodeFinanceProtocolVersion(id, protocolID, version, configJSON, checksum, status, validationJSON, publishedAt, createdBy, createdAt.Time)
}
func decodeFinanceProtocolVersion(id, protocolID int64, version int, configJSON []byte, checksum, status string, validationJSON []byte, publishedAt sql.NullTime, createdBy sql.NullInt64, createdAt time.Time) (*service.UpstreamFinanceProtocolVersion, error) {
	var config service.FinanceProtocolConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, err
	}
	var validation service.FinanceProtocolValidationResult
	if len(validationJSON) > 0 {
		if err := json.Unmarshal(validationJSON, &validation); err != nil {
			return nil, err
		}
	}
	return &service.UpstreamFinanceProtocolVersion{ID: id, ProtocolID: protocolID, Version: version, Config: config, Checksum: checksum, ValidationStatus: status, ValidationResult: validation, PublishedAt: nullTimePointer(publishedAt), CreatedBy: nullInt64Pointer(createdBy), CreatedAt: createdAt}, nil
}
func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}
func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}
func isFinanceProtocolUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
