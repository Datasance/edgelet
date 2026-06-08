package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const singletonRowID = 1

// UpsertEdgeGuardSignature stores the current edgeguard hardware signature JWT.
func (d *DB) UpsertEdgeGuardSignature(jwt string) error {
	if d.Conn() == nil {
		return errors.New("sqlite not open")
	}

	_, err := d.Conn().Exec(
		`INSERT INTO agent_edgeguard_signature (id, signature_jwt, updated_at)
		 VALUES (?, ?, strftime('%s','now'))
		 ON CONFLICT(id) DO UPDATE SET
		   signature_jwt = excluded.signature_jwt,
		   updated_at = strftime('%s','now')`,
		singletonRowID, jwt,
	)
	if err != nil {
		return fmt.Errorf("upsert edgeguard signature: %w", err)
	}

	return nil
}

// GetEdgeGuardSignature reads the current edgeguard hardware signature JWT.
// Returns found=false when there is no stored signature.
func (d *DB) GetEdgeGuardSignature() (signature string, found bool, err error) {
	if d.Conn() == nil {
		return "", false, errors.New("sqlite not open")
	}

	err = d.Conn().QueryRow(
		`SELECT signature_jwt FROM agent_edgeguard_signature WHERE id = ?`,
		singletonRowID,
	).Scan(&signature)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get edgeguard signature: %w", err)
	}

	return signature, true, nil
}

// DeleteEdgeGuardSignature removes the stored edgeguard signature JWT.
func (d *DB) DeleteEdgeGuardSignature() error {
	if d.Conn() == nil {
		return errors.New("sqlite not open")
	}

	if _, err := d.Conn().Exec(
		`DELETE FROM agent_edgeguard_signature WHERE id = ?`,
		singletonRowID,
	); err != nil {
		return fmt.Errorf("delete edgeguard signature: %w", err)
	}

	return nil
}

// UpsertAgentPrivateKey stores the provisioned private key payload (base64 JWK).
func (d *DB) UpsertAgentPrivateKey(privateKeyB64 string) error {
	if d.Conn() == nil {
		return errors.New("sqlite not open")
	}

	_, err := d.Conn().Exec(
		`INSERT INTO agent_credentials (id, private_key_b64, updated_at)
		 VALUES (?, ?, strftime('%s','now'))
		 ON CONFLICT(id) DO UPDATE SET
		   private_key_b64 = excluded.private_key_b64,
		   updated_at = strftime('%s','now')`,
		singletonRowID, privateKeyB64,
	)
	if err != nil {
		return fmt.Errorf("upsert agent private key: %w", err)
	}

	return nil
}

// GetAgentPrivateKey reads the provisioned private key payload (base64 JWK).
// Returns found=false when there is no stored key.
func (d *DB) GetAgentPrivateKey() (privateKeyB64 string, found bool, err error) {
	if d.Conn() == nil {
		return "", false, errors.New("sqlite not open")
	}

	err = d.Conn().QueryRow(
		`SELECT private_key_b64 FROM agent_credentials WHERE id = ?`,
		singletonRowID,
	).Scan(&privateKeyB64)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get agent private key: %w", err)
	}

	return privateKeyB64, true, nil
}

// DeleteAgentPrivateKey removes the provisioned private key payload.
func (d *DB) DeleteAgentPrivateKey() error {
	if d.Conn() == nil {
		return errors.New("sqlite not open")
	}

	if _, err := d.Conn().Exec(
		`DELETE FROM agent_credentials WHERE id = ?`,
		singletonRowID,
	); err != nil {
		return fmt.Errorf("delete agent private key: %w", err)
	}

	return nil
}
