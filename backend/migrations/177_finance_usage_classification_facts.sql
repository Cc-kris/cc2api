CREATE TABLE IF NOT EXISTS user_promotion_credit_balances (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    remaining_amount DECIMAL(20,10) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_promotion_credit_balances_nonnegative CHECK (remaining_amount >= 0)
);

CREATE OR REPLACE FUNCTION add_promotion_credit_from_usage()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO user_promotion_credit_balances(user_id, remaining_amount, updated_at)
    VALUES(NEW.user_id, GREATEST(NEW.bonus_amount, 0), NOW())
    ON CONFLICT(user_id) DO UPDATE
    SET remaining_amount = user_promotion_credit_balances.remaining_amount + EXCLUDED.remaining_amount,
        updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_promo_code_usage_finance_credit ON promo_code_usages;
CREATE TRIGGER trg_promo_code_usage_finance_credit
AFTER INSERT ON promo_code_usages
FOR EACH ROW EXECUTE FUNCTION add_promotion_credit_from_usage();

ALTER TABLE usage_billing_dedup
    ADD COLUMN IF NOT EXISTS finance_business_type VARCHAR(30) NOT NULL DEFAULT 'balance',
    ADD COLUMN IF NOT EXISTS promotion_credit_used DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS finance_excluded BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS finance_exclusion_reason VARCHAR(100);

ALTER TABLE usage_billing_dedup_archive
    ADD COLUMN IF NOT EXISTS finance_business_type VARCHAR(30) NOT NULL DEFAULT 'balance',
    ADD COLUMN IF NOT EXISTS promotion_credit_used DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS finance_excluded BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS finance_exclusion_reason VARCHAR(100);

ALTER TABLE usage_billing_dedup
    DROP CONSTRAINT IF EXISTS usage_billing_dedup_finance_business_type_check;
ALTER TABLE usage_billing_dedup
    ADD CONSTRAINT usage_billing_dedup_finance_business_type_check
    CHECK (finance_business_type IN ('balance','subscription','promotion','admin'));

ALTER TABLE usage_billing_dedup_archive
    DROP CONSTRAINT IF EXISTS usage_billing_dedup_archive_finance_business_type_check;
ALTER TABLE usage_billing_dedup_archive
    ADD CONSTRAINT usage_billing_dedup_archive_finance_business_type_check
    CHECK (finance_business_type IN ('balance','subscription','promotion','admin'));
