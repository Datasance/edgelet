package models

// ControllerStatus represents the controller connection status
type ControllerStatus string

const (
	ControllerStatusOK                ControllerStatus = "OK"
	ControllerStatusNotConnected      ControllerStatus = "NOT_CONNECTED"
	ControllerStatusNotProvisioned    ControllerStatus = "NOT_PROVISIONED"
	ControllerStatusBrokenCertificate ControllerStatus = "BROKEN_CERTIFICATE"
	ControllerStatusBadRequest        ControllerStatus = "BAD_REQUEST"
	ControllerStatusUnauthorized      ControllerStatus = "UNAUTHORIZED"
	ControllerStatusNotFound          ControllerStatus = "NOT_FOUND"
	ControllerStatusInternalError     ControllerStatus = "INTERNAL_ERROR"
)

// FieldAgentStatus represents the Field Agent status
type FieldAgentStatus struct {
	ControllerStatus   ControllerStatus `json:"controllerStatus" yaml:"controllerStatus"`
	LastCommandTime    int64            `json:"lastCommandTime" yaml:"lastCommandTime"`
	ControllerVerified bool             `json:"controllerVerified" yaml:"controllerVerified"`
	ReadyToUpgrade     bool             `json:"readyToUpgrade" yaml:"readyToUpgrade"`
	ReadyToRollback    bool             `json:"readyToRollback" yaml:"readyToRollback"`
}

// NewFieldAgentStatus creates a new FieldAgentStatus with default values
func NewFieldAgentStatus() *FieldAgentStatus {
	return &FieldAgentStatus{
		ControllerStatus: ControllerStatusNotConnected,
	}
}
