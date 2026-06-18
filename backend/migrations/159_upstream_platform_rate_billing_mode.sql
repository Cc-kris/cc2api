ALTER TABLE upstream_platform_rates
  ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) NOT NULL DEFAULT 'token';

UPDATE upstream_platform_rates
SET billing_mode = CASE WHEN COALESCE(image_unit_price, 0) > 0 THEN 'image_per_use' ELSE 'token' END
WHERE billing_mode IS NULL OR billing_mode = '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_upstream_platform_rates_billing_mode'
  ) THEN
    ALTER TABLE upstream_platform_rates
      ADD CONSTRAINT chk_upstream_platform_rates_billing_mode
      CHECK (billing_mode IN ('token', 'image_per_use'));
  END IF;
END $$;

COMMENT ON COLUMN upstream_platform_rates.billing_mode IS 'Cost mode for this platform: token multiplier or image per-use fixed cost.';
