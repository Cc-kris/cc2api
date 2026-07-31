-- Keep FX evidence intervals non-overlapping for every currency.
-- Application writes also close the previous interval, while this trigger
-- protects direct SQL/import paths and concurrent writers.
CREATE OR REPLACE FUNCTION finance_fx_rate_reject_overlap()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM finance_fx_rate_versions existing
         WHERE existing.currency = NEW.currency
           AND existing.id <> COALESCE(NEW.id, 0)
           AND tstzrange(
                   existing.effective_from,
                   COALESCE(existing.effective_to, 'infinity'::timestamptz),
                   '[)'
               ) && tstzrange(
                   NEW.effective_from,
                   COALESCE(NEW.effective_to, 'infinity'::timestamptz),
                   '[)'
               )
    ) THEN
        RAISE EXCEPTION 'finance FX rate effective range overlaps an existing version for currency %', NEW.currency
            USING ERRCODE = '23P01';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS finance_fx_rate_no_overlap ON finance_fx_rate_versions;
CREATE TRIGGER finance_fx_rate_no_overlap
    BEFORE INSERT OR UPDATE OF currency, effective_from, effective_to
    ON finance_fx_rate_versions
    FOR EACH ROW
    EXECUTE FUNCTION finance_fx_rate_reject_overlap();
