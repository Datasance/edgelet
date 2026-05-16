package models

// ServiceAccountToken represents persisted token metadata for LocalAPI v3.
type ServiceAccountToken struct {
	ID                 string
	TokenUse           string
	PrincipalType      string
	Subject            string
	MicroserviceUUID   string
	ApplicationName    string
	ServiceAccountName string
	RoleRefKind        string
	RoleRefName        string
	RBACVersion        string
	RulesByGroupJSON   string
	ClaimsJSON         string
	Issuer             string
	Audience           string
	Alg                string
	JTI                string
	TokenSHA256        string
	IssuedAt           int64
	NotBefore          int64
	ExpiresAt          int64
	RevokedAt          *int64
	RotatedFromJTI     string
}
