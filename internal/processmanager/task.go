package processmanager

// TaskAction represents the type of container task
type TaskAction string

const (
	TaskActionAdd               TaskAction = "ADD"
	TaskActionUpdate            TaskAction = "UPDATE"
	TaskActionRemove            TaskAction = "REMOVE"
	TaskActionRemoveWithCleanup TaskAction = "REMOVE_WITH_CLEAN_UP"
	TaskActionStop              TaskAction = "STOP"
	TaskActionCreateExec        TaskAction = "CREATE_EXEC"
	TaskActionGetExecStatus     TaskAction = "GET_EXEC_STATUS"
	TaskActionKillExec          TaskAction = "KILL_EXEC"
)

// ContainerTask represents a task to be executed on a container
type ContainerTask struct {
	Action           TaskAction
	MicroserviceUUID string
	Retries          int
	Command          []string
	ExecID           string
	// Callback for exec sessions (will be implemented when we add exec support)
	// Callback ExecSessionCallback
}

// NewContainerTask creates a new ContainerTask
func NewContainerTask(action TaskAction, microserviceUUID string) *ContainerTask {
	return &ContainerTask{
		Action:           action,
		MicroserviceUUID: microserviceUUID,
		Retries:          0,
	}
}

// NewContainerTaskWithExec creates a new ContainerTask for exec operations
func NewContainerTaskWithExec(action TaskAction, microserviceUUID string, command []string) *ContainerTask {
	return &ContainerTask{
		Action:           action,
		MicroserviceUUID: microserviceUUID,
		Command:          command,
		Retries:          0,
	}
}

// IncrementRetries increments the retry count
func (t *ContainerTask) IncrementRetries() {
	t.Retries++
}
