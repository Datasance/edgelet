-- Enforce local deploy identity uniqueness for idempotent apply by local app/name.

UPDATE local_deployed_microservices
SET application_name = 'local'
WHERE TRIM(COALESCE(application_name, '')) = '';

DELETE FROM local_deployed_microservices AS older
WHERE EXISTS (
  SELECT 1
  FROM local_deployed_microservices AS newer
  WHERE LOWER(TRIM(COALESCE(newer.application_name, ''))) = LOWER(TRIM(COALESCE(older.application_name, '')))
    AND LOWER(TRIM(COALESCE(newer.microservice_name, ''))) = LOWER(TRIM(COALESCE(older.microservice_name, '')))
    AND (
      COALESCE(newer.updated_at, newer.created_at, 0) > COALESCE(older.updated_at, older.created_at, 0)
      OR (
        COALESCE(newer.updated_at, newer.created_at, 0) = COALESCE(older.updated_at, older.created_at, 0)
        AND newer.rowid > older.rowid
      )
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_local_deployed_microservices_unique_app_name
  ON local_deployed_microservices(application_name COLLATE NOCASE, microservice_name COLLATE NOCASE)
  WHERE COALESCE(deleted_at, 0) = 0;
