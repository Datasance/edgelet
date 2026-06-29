package fieldagent

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
)

func TestRescheduleUpgradeScanIfFrequencyChanged(t *testing.T) {
	fa := &FieldAgent{
		config: config.GetInstance(),
		state:  NewState(),
	}
	fa.ensureUpgradeScanRescheduleChan()
	fa.setAppliedUpgradeScanFrequency(24)

	cfg := config.GetInstance()
	original := cfg.UpgradeScanFrequency
	cfg.UpgradeScanFrequency = 1
	t.Cleanup(func() {
		cfg.UpgradeScanFrequency = original
	})

	fa.rescheduleUpgradeScanIfFrequencyChanged()

	select {
	case <-fa.upgradeScanReschedule:
	case <-time.After(time.Second):
		t.Fatal("expected upgrade scan reschedule notification")
	}

	fa.setAppliedUpgradeScanFrequency(1)
	fa.rescheduleUpgradeScanIfFrequencyChanged()
	select {
	case <-fa.upgradeScanReschedule:
		t.Fatal("expected no reschedule when frequency already applied")
	default:
	}
}

func TestWaitForDaemonOperational(t *testing.T) {
	sr := statusreporter.GetInstance()
	sr.UpdateSupervisorStatus(func(s *models.SupervisorStatus) {
		s.SetDaemonStatus(models.ModuleStatusStarting)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fa := &FieldAgent{config: config.GetInstance(), state: NewState(), ctx: ctx}

	done := make(chan bool, 1)
	go func() {
		done <- fa.waitForDaemonOperational(ctx)
	}()

	time.Sleep(upgradeScanPollInterval * 2)
	sr.UpdateSupervisorStatus(func(s *models.SupervisorStatus) {
		s.SetDaemonStatus(models.ModuleStatusRunning)
	})

	select {
	case ok := <-done:
		if !ok {
			t.Fatal("expected waitForDaemonOperational to succeed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for daemon operational")
	}
}

func TestUpgradeScanFrequencyHours(t *testing.T) {
	if got := upgradeScanFrequencyHours(0); got != defaultUpgradeScanHours {
		t.Fatalf("zero hours: got %d want %d", got, defaultUpgradeScanHours)
	}
	if got := upgradeScanFrequencyHours(6); got != 6 {
		t.Fatalf("positive hours: got %d want 6", got)
	}
}
