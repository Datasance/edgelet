ALTER TABLE local_deployed_microservices ADD COLUMN last_reconcile_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE local_deployed_microservices ADD COLUMN last_start_attempt_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE local_deployed_microservices ADD COLUMN failure_count INTEGER NOT NULL DEFAULT 0;

UPDATE local_deployed_microservices
SET last_reconcile_at = COALESCE(updated_at, strftime('%s','now'))
WHERE COALESCE(last_reconcile_at, 0) <= 0;

UPDATE local_deployed_microservices
SET failure_count = CASE
  WHEN LOWER(TRIM(COALESCE(runtime_state, ''))) IN ('failed', 'stuck_in_restart') THEN 1
  ELSE 0
END
WHERE COALESCE(failure_count, 0) < 0 OR COALESCE(failure_count, 0) = 0;

CREATE INDEX IF NOT EXISTS idx_local_deployed_microservices_runtime_reconcile
  ON local_deployed_microservices(desired_state, runtime_state, failure_count);
