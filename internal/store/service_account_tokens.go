package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/datasance/edgelet/internal/models"
)

// UpsertServiceAccountToken upserts token metadata.
func (d *DB) UpsertServiceAccountToken(token *models.ServiceAccountToken) error {
	if token == nil {
		return fmt.Errorf("token is nil")
	}

	_, err := d.Conn().Exec(`INSERT OR REPLACE INTO local_service_account_tokens (
		id, token_use, principal_type, subject, microservice_uuid, application_name,
		service_account_name, role_ref_kind, role_ref_name, rbac_version, rules_by_group_json, claims_json,
		issuer, audience, alg, jti, token_sha256, issued_at, not_before, expires_at,
		revoked_at, rotated_from_jti, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		token.ID, token.TokenUse, token.PrincipalType, token.Subject, token.MicroserviceUUID, token.ApplicationName,
		token.ServiceAccountName, token.RoleRefKind, token.RoleRefName, token.RBACVersion, token.RulesByGroupJSON, token.ClaimsJSON,
		token.Issuer, token.Audience, token.Alg, token.JTI, token.TokenSHA256, token.IssuedAt, token.NotBefore, token.ExpiresAt,
		token.RevokedAt, token.RotatedFromJTI, time.Now().Unix(), time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert service_account_token: %w", err)
	}
	return nil
}

// RevokeServiceAccountToken revokes a token by JTI.
func (d *DB) RevokeServiceAccountToken(jti string, revokedAt int64) error {
	if jti == "" {
		return fmt.Errorf("jti is required")
	}
	if revokedAt == 0 {
		revokedAt = time.Now().Unix()
	}
	_, err := d.Conn().Exec(`UPDATE local_service_account_tokens SET revoked_at = ?, updated_at = ? WHERE jti = ?`, revokedAt, time.Now().Unix(), jti)
	return err
}

// ListServiceAccountTokens lists token metadata.
func (d *DB) ListServiceAccountTokens() ([]*models.ServiceAccountToken, error) {
	rows, err := d.Conn().Query(`SELECT
		id, token_use, principal_type, subject, microservice_uuid, application_name, service_account_name, role_ref_kind, role_ref_name,
		rbac_version, rules_by_group_json, claims_json,
		issuer, audience, alg, jti, token_sha256, issued_at, not_before, expires_at,
		revoked_at, rotated_from_jti
		FROM local_service_account_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query local_service_account_tokens: %w", err)
	}
	defer rows.Close()

	var result []*models.ServiceAccountToken
	for rows.Next() {
		item := &models.ServiceAccountToken{}
		var revoked sql.NullInt64
		if scanErr := rows.Scan(
			&item.ID, &item.TokenUse, &item.PrincipalType, &item.Subject, &item.MicroserviceUUID, &item.ApplicationName, &item.ServiceAccountName, &item.RoleRefKind, &item.RoleRefName,
			&item.RBACVersion, &item.RulesByGroupJSON, &item.ClaimsJSON,
			&item.Issuer, &item.Audience, &item.Alg, &item.JTI, &item.TokenSHA256, &item.IssuedAt, &item.NotBefore, &item.ExpiresAt,
			&revoked, &item.RotatedFromJTI,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan local_service_account_tokens row: %w", scanErr)
		}
		if revoked.Valid {
			item.RevokedAt = &revoked.Int64
		}
		result = append(result, item)
	}
	if result == nil {
		result = make([]*models.ServiceAccountToken, 0)
	}
	return result, rows.Err()
}

// ListActiveServiceAccountTokens lists non-revoked token metadata.
func (d *DB) ListActiveServiceAccountTokens() ([]*models.ServiceAccountToken, error) {
	rows, err := d.Conn().Query(`SELECT
		id, token_use, principal_type, subject, microservice_uuid, application_name, service_account_name, role_ref_kind, role_ref_name,
		rbac_version, rules_by_group_json, claims_json,
		issuer, audience, alg, jti, token_sha256, issued_at, not_before, expires_at,
		revoked_at, rotated_from_jti
		FROM local_service_account_tokens WHERE revoked_at IS NULL ORDER BY issued_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query active local_service_account_tokens: %w", err)
	}
	defer rows.Close()

	var result []*models.ServiceAccountToken
	for rows.Next() {
		item := &models.ServiceAccountToken{}
		var revoked sql.NullInt64
		if scanErr := rows.Scan(
			&item.ID, &item.TokenUse, &item.PrincipalType, &item.Subject, &item.MicroserviceUUID, &item.ApplicationName, &item.ServiceAccountName, &item.RoleRefKind, &item.RoleRefName,
			&item.RBACVersion, &item.RulesByGroupJSON, &item.ClaimsJSON,
			&item.Issuer, &item.Audience, &item.Alg, &item.JTI, &item.TokenSHA256, &item.IssuedAt, &item.NotBefore, &item.ExpiresAt,
			&revoked, &item.RotatedFromJTI,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan active local_service_account_tokens row: %w", scanErr)
		}
		if revoked.Valid {
			item.RevokedAt = &revoked.Int64
		}
		result = append(result, item)
	}
	if result == nil {
		result = make([]*models.ServiceAccountToken, 0)
	}
	return result, rows.Err()
}

// RevokeAllServiceAccountTokens marks all tokens revoked.
func (d *DB) RevokeAllServiceAccountTokens(revokedAt int64) error {
	if revokedAt == 0 {
		revokedAt = time.Now().Unix()
	}
	_, err := d.Conn().Exec(`UPDATE local_service_account_tokens SET revoked_at = ?, updated_at = ? WHERE revoked_at IS NULL`, revokedAt, time.Now().Unix())
	return err
}

// ClearServiceAccountTokens removes all token metadata rows.
func (d *DB) ClearServiceAccountTokens() error {
	_, err := d.Conn().Exec(`DELETE FROM local_service_account_tokens`)
	return err
}
