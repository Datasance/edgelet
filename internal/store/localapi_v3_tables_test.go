package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

func openStoreForLocalAPIV3Tests(t *testing.T) *DB {
	t.Helper()
	db := GetInstance()
	if err := db.Open(t.TempDir()); err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMigration004CreatesTables(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	requiredTables := []string{"service_account_tokens", "local_deployed_microservices", "local_runtime_classes"}
	for _, table := range requiredTables {
		table := table
		t.Run(table, func(t *testing.T) {
			var name string
			err := db.Conn().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
			if err != nil {
				t.Fatalf("expected table %s to exist, got error: %v", table, err)
			}
		})
	}
}

func TestMigration010CreatesLocalRuntimeClassesSchema(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	requiredColumns := []string{
		"name",
		"handler",
		"created_at",
		"updated_at",
	}
	removedColumns := []string{
		"binary_path",
		"runtime_name",
		"runtime_local_name",
	}

	rows, err := db.Conn().Query(`PRAGMA table_info(local_runtime_classes)`)
	if err != nil {
		t.Fatalf("failed to inspect local_runtime_classes schema: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue interface{}
			pk        int
		)
		if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); scanErr != nil {
			t.Fatalf("failed to scan pragma row: %v", scanErr)
		}
		seen[name] = true
	}
	for _, col := range requiredColumns {
		if !seen[col] {
			t.Fatalf("expected column %q to exist after migration 010", col)
		}
	}
	for _, col := range removedColumns {
		if seen[col] {
			t.Fatalf("expected column %q to be removed in migration 010", col)
		}
	}
}

func TestMigration010IsDeterministicAcrossReopen(t *testing.T) {
	dbDir := t.TempDir()
	db := GetInstance()
	if err := db.Open(dbDir); err != nil {
		t.Fatalf("failed first open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("failed first close: %v", err)
	}
	if err := db.Open(dbDir); err != nil {
		t.Fatalf("failed second open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var name string
	if err := db.Conn().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, "local_runtime_classes").Scan(&name); err != nil {
		t.Fatalf("expected local_runtime_classes table after reopen migration: %v", err)
	}
	if name != "local_runtime_classes" {
		t.Fatalf("unexpected table name after reopen: %q", name)
	}

	dbPath := filepath.Join(dbDir, dbFileName)
	if dbPath == "" {
		t.Fatal("expected sqlite db path to be set")
	}
}

func TestMigration007AddsLocalDeploymentLifecycleColumns(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	requiredColumns := []string{
		"desired_state",
		"runtime_state",
		"last_error",
		"restart_count",
		"last_transition_at",
		"deleted_at",
		"generation",
		"observed_generation",
	}

	rows, err := db.Conn().Query(`PRAGMA table_info(local_deployed_microservices)`)
	if err != nil {
		t.Fatalf("failed to inspect local_deployed_microservices schema: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue interface{}
			pk        int
		)
		if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); scanErr != nil {
			t.Fatalf("failed to scan pragma row: %v", scanErr)
		}
		seen[name] = true
	}

	for _, col := range requiredColumns {
		if !seen[col] {
			t.Fatalf("expected column %q to exist after migration 007", col)
		}
	}
}

func TestMigration008AddsUniqueLocalDeploymentAppNameIndex(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)
	var name string
	err := db.Conn().QueryRow(`
		SELECT name
		FROM sqlite_master
		WHERE type='index'
		  AND name='idx_local_deployed_microservices_unique_app_name'`).Scan(&name)
	if err != nil {
		t.Fatalf("expected unique app/name index to exist after migration 008: %v", err)
	}
}

func TestMigration009AddsLocalReconcileTrackingColumns(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	requiredColumns := []string{
		"last_reconcile_at",
		"last_start_attempt_at",
		"failure_count",
	}

	rows, err := db.Conn().Query(`PRAGMA table_info(local_deployed_microservices)`)
	if err != nil {
		t.Fatalf("failed to inspect local_deployed_microservices schema: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue interface{}
			pk        int
		)
		if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); scanErr != nil {
			t.Fatalf("failed to scan pragma row: %v", scanErr)
		}
		seen[name] = true
	}
	for _, col := range requiredColumns {
		if !seen[col] {
			t.Fatalf("expected column %q to exist after migration 009", col)
		}
	}
}

func TestServiceAccountTokenCRUD(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	token := &models.ServiceAccountToken{
		ID:                 "tok-1",
		TokenUse:           "serviceaccount",
		PrincipalType:      "serviceaccount",
		Subject:            "system:serviceaccount:app:ms",
		MicroserviceUUID:   "ms-uuid",
		ApplicationName:    "app",
		ServiceAccountName: "sa-1",
		RoleRefKind:        "Role",
		RoleRefName:        "role-1",
		RBACVersion:        "v1",
		RulesByGroupJSON:   `{"edgelet.iofog.org/v1":[{"resources":["logs"],"verbs":["get"]}]}`,
		ClaimsJSON:         `{}`,
		Issuer:             "https://edgelet.default.svc.bridge.local",
		Audience:           "https://edgelet.default.svc.bridge.local",
		Alg:                "EdDSA",
		JTI:                "jti-1",
		TokenSHA256:        "sha256-1",
		IssuedAt:           time.Now().Unix(),
		NotBefore:          time.Now().Unix(),
		ExpiresAt:          time.Now().Add(time.Hour).Unix(),
	}
	if err := db.UpsertServiceAccountToken(token); err != nil {
		t.Fatalf("failed to upsert token: %v", err)
	}

	items, err := db.ListServiceAccountTokens()
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 token row, got %d", len(items))
	}
	if items[0].JTI != "jti-1" {
		t.Fatalf("unexpected token jti: %s", items[0].JTI)
	}

	if err := db.RevokeServiceAccountToken("jti-1", time.Now().Unix()); err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}
	items, err = db.ListServiceAccountTokens()
	if err != nil {
		t.Fatalf("failed to list tokens after revoke: %v", err)
	}
	if items[0].RevokedAt == nil {
		t.Fatal("expected revoked_at to be set after revocation")
	}
}

func TestLocalDeployedMicroserviceCRUD(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	ms := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-1",
		ApplicationName:  "",
		MicroserviceName: "edge-processor",
		SourceName:       "local-apply",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nginx:latest",
		State:            "running",
		ContainerID:      "cid-1",
	}
	if err := db.UpsertLocalDeployedMicroservice(ms); err != nil {
		t.Fatalf("failed to upsert local deployment: %v", err)
	}

	list, err := db.ListLocalDeployedMicroservices()
	if err != nil {
		t.Fatalf("failed to list local deployments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 local deployment row, got %d", len(list))
	}

	got, err := db.GetLocalDeployedMicroservice("local-1")
	if err != nil {
		t.Fatalf("failed to get local deployment: %v", err)
	}
	if got.ApplicationName != "local" {
		t.Fatalf("expected application_name to normalize to local, got %q", got.ApplicationName)
	}
	if got.MicroserviceName != "edge-processor" {
		t.Fatalf("unexpected microservice name: %s", got.MicroserviceName)
	}
	if got.DesiredState != "running" {
		t.Fatalf("expected desired_state running, got %q", got.DesiredState)
	}
	if got.RuntimeState != "running" {
		t.Fatalf("expected runtime_state running, got %q", got.RuntimeState)
	}
	if got.Generation != 1 {
		t.Fatalf("expected generation=1, got %d", got.Generation)
	}
	if got.LastReconcileAt != 0 {
		t.Fatalf("expected last_reconcile_at=0 default, got %d", got.LastReconcileAt)
	}
	if got.LastStartAttemptAt != 0 {
		t.Fatalf("expected last_start_attempt_at=0 default, got %d", got.LastStartAttemptAt)
	}
	if got.FailureCount != 0 {
		t.Fatalf("expected failure_count=0 default, got %d", got.FailureCount)
	}
	if got.LastTransitionAt <= 0 {
		t.Fatalf("expected last_transition_at > 0, got %d", got.LastTransitionAt)
	}

	if err := db.DeleteLocalDeployedMicroservice("local-1"); err != nil {
		t.Fatalf("failed to delete local deployment: %v", err)
	}
	_, err = db.GetLocalDeployedMicroservice("local-1")
	if err != sql.ErrNoRows {
		t.Fatalf("unexpected error after deletion: %v", err)
	}
}

func TestLocalDeployedMicroserviceUniqueByAppName(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	first := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-a",
		ApplicationName:  "local",
		MicroserviceName: "router",
		SourceName:       "local-apply",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nginx:latest",
		State:            "running",
	}
	if err := db.UpsertLocalDeployedMicroservice(first); err != nil {
		t.Fatalf("failed to insert first local deployment: %v", err)
	}

	second := &models.LocalDeployedMicroservice{
		LocalUUID:        "local-b",
		ApplicationName:  "LOCAL",
		MicroserviceName: "router",
		SourceName:       "local-apply",
		ManifestYAML:     "kind: Microservice",
		ImageName:        "nginx:latest",
		State:            "running",
	}
	if err := db.UpsertLocalDeployedMicroservice(second); err == nil {
		t.Fatalf("expected unique constraint violation for duplicate local app/name")
	}
}

func TestLocalRuntimeClassCRUD(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	rc := &models.LocalRuntimeClass{
		Name:        "edgelet",
		Handler:     "edgelet",
		RuntimeName: "edgelet",
	}
	if err := db.UpsertLocalRuntimeClass(rc); err != nil {
		t.Fatalf("failed to upsert local runtime class: %v", err)
	}

	items, err := db.ListLocalRuntimeClasses()
	if err != nil {
		t.Fatalf("failed to list local runtime classes: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 runtime class row, got %d", len(items))
	}
	if items[0].RuntimeName != "edgelet" {
		t.Fatalf("unexpected runtime name: %+v", items[0])
	}

	got, err := db.GetLocalRuntimeClass("edgelet")
	if err != nil {
		t.Fatalf("failed to get runtime class: %v", err)
	}
	if got.Handler != "edgelet" {
		t.Fatalf("unexpected handler: %q", got.Handler)
	}

	if err := db.DeleteLocalRuntimeClass("edgelet"); err != nil {
		t.Fatalf("failed to delete runtime class: %v", err)
	}
	_, err = db.GetLocalRuntimeClass("edgelet")
	if err != sql.ErrNoRows {
		t.Fatalf("unexpected error after runtime class deletion: %v", err)
	}
}
