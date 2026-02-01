package models

// MicroserviceState represents the state of a microservice
type MicroserviceState string

const (
	MicroserviceStateQueued            MicroserviceState = "QUEUED"
	MicroserviceStatePulling           MicroserviceState = "PULLING"
	MicroserviceStateCreating          MicroserviceState = "CREATING"
	MicroserviceStateCreated           MicroserviceState = "CREATED"
	MicroserviceStateStarting          MicroserviceState = "STARTING"
	MicroserviceStateRunning           MicroserviceState = "RUNNING"
	MicroserviceStateUpdating          MicroserviceState = "UPDATING"
	MicroserviceStateRestarting        MicroserviceState = "RESTARTING"
	MicroserviceStateExiting           MicroserviceState = "EXITING"
	MicroserviceStateStuckInRestart    MicroserviceState = "STUCK_IN_RESTART"
	MicroserviceStateFailed            MicroserviceState = "FAILED"
	MicroserviceStateMarkedForDeletion MicroserviceState = "MARKED_FOR_DELETION"
	MicroserviceStateDeleting          MicroserviceState = "DELETING"
	MicroserviceStateDeleted           MicroserviceState = "DELETED"
	MicroserviceStateStopping          MicroserviceState = "STOPPING"
	MicroserviceStateStopped           MicroserviceState = "STOPPED"
	MicroserviceStateUnknown           MicroserviceState = "UNKNOWN"
)

// FromText converts a string to MicroserviceState
func MicroserviceStateFromText(value string) MicroserviceState {
	// Convert to uppercase to match Java's valueOf behavior
	upper := ""
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			upper += string(r - 32)
		} else {
			upper += string(r)
		}
	}

	switch upper {
	case "QUEUED":
		return MicroserviceStateQueued
	case "PULLING":
		return MicroserviceStatePulling
	case "CREATING":
		return MicroserviceStateCreating
	case "CREATED":
		return MicroserviceStateCreated
	case "STARTING":
		return MicroserviceStateStarting
	case "RUNNING":
		return MicroserviceStateRunning
	case "UPDATING":
		return MicroserviceStateUpdating
	case "RESTARTING":
		return MicroserviceStateRestarting
	case "EXITING":
		return MicroserviceStateExiting
	case "STUCK_IN_RESTART":
		return MicroserviceStateStuckInRestart
	case "FAILED":
		return MicroserviceStateFailed
	case "MARKED_FOR_DELETION":
		return MicroserviceStateMarkedForDeletion
	case "DELETING":
		return MicroserviceStateDeleting
	case "DELETED":
		return MicroserviceStateDeleted
	case "STOPPING":
		return MicroserviceStateStopping
	case "STOPPED":
		return MicroserviceStateStopped
	default:
		return MicroserviceStateUnknown
	}
}
