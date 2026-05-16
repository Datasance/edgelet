ALTER TABLE local_deployed_microservices ADD COLUMN desired_state TEXT NOT NULL DEFAULT 'running';
ALTER TABLE local_deployed_microservices ADD COLUMN runtime_state TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE local_deployed_microservices ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE local_deployed_microservices ADD COLUMN restart_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE local_deployed_microservices ADD COLUMN last_transition_at INTEGER NOT NULL DEFAULT (strftime('%s','now'));
ALTER TABLE local_deployed_microservices ADD COLUMN deleted_at INTEGER;
ALTER TABLE local_deployed_microservices ADD COLUMN generation INTEGER NOT NULL DEFAULT 1;
ALTER TABLE local_deployed_microservices ADD COLUMN observed_generation INTEGER NOT NULL DEFAULT 0;

UPDATE local_deployed_microservices
SET application_name = 'local'
WHERE TRIM(COALESCE(application_name, '')) = '';

UPDATE local_deployed_microservices
SET runtime_state = state
WHERE TRIM(COALESCE(state, '')) <> ''
  AND TRIM(COALESCE(runtime_state, '')) = '';

UPDATE local_deployed_microservices
SET runtime_state = 'unknown'
WHERE TRIM(COALESCE(runtime_state, '')) = '';

UPDATE local_deployed_microservices
SET desired_state = 'running'
WHERE TRIM(COALESCE(desired_state, '')) = '';

UPDATE local_deployed_microservices
SET state = runtime_state
WHERE TRIM(COALESCE(state, '')) = '';

UPDATE local_deployed_microservices
SET last_transition_at = COALESCE(updated_at, strftime('%s','now'))
WHERE COALESCE(last_transition_at, 0) <= 0;

CREATE INDEX IF NOT EXISTS idx_local_deployed_microservices_desired_runtime
  ON local_deployed_microservices(desired_state, runtime_state);

CREATE INDEX IF NOT EXISTS idx_local_deployed_microservices_name_active
  ON local_deployed_microservices(application_name, microservice_name, deleted_at);
