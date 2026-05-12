package store

import (
	"database/sql"
	"testing"
	"time"

	"github.com/eclipse-iofog/agent/internal/models"
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

	requiredTables := []string{"service_account_tokens", "local_deployed_microservices"}
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

func TestServiceAccountTokenCRUD(t *testing.T) {
	db := openStoreForLocalAPIV3Tests(t)

	token := &models.ServiceAccountToken{
		ID:               "tok-1",
		TokenUse:         "serviceaccount",
		PrincipalType:    "serviceaccount",
		Subject:          "system:serviceaccount:app:ms",
		MicroserviceUUID: "ms-uuid",
		ApplicationName:  "app",
		ServiceAccountName: "sa-1",
		RoleRefKind:      "Role",
		RoleRefName:      "role-1",
		RBACVersion:      "v1",
		RulesByGroupJSON: `{"agent.datasance.com/v3":[{"resources":["logs"],"verbs":["get"]}]}`,
		ClaimsJSON:       `{}`,
		Issuer:           "https://iofog.default.svc.bridge.local",
		Audience:         "https://iofog.default.svc.bridge.local",
		Alg:              "EdDSA",
		JTI:              "jti-1",
		TokenSHA256:      "sha256-1",
		IssuedAt:         time.Now().Unix(),
		NotBefore:        time.Now().Unix(),
		ExpiresAt:        time.Now().Add(time.Hour).Unix(),
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
	if got.MicroserviceName != "edge-processor" {
		t.Fatalf("unexpected microservice name: %s", got.MicroserviceName)
	}

	if err := db.DeleteLocalDeployedMicroservice("local-1"); err != nil {
		t.Fatalf("failed to delete local deployment: %v", err)
	}
	_, err = db.GetLocalDeployedMicroservice("local-1")
	if err != sql.ErrNoRows {
		t.Fatalf("unexpected error after deletion: %v", err)
	}
}
