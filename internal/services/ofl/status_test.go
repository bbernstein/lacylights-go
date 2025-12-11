package ofl

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestStatusTracker_NewStatusTracker(t *testing.T) {
	tracker := NewStatusTracker()
	if tracker == nil {
		t.Fatal("NewStatusTracker returned nil")
	}

	status := tracker.GetStatus()
	if status.IsImporting {
		t.Error("New tracker should not be importing")
	}
	if status.Phase != PhaseIdle {
		t.Errorf("New tracker should be in IDLE phase, got %s", status.Phase)
	}
}

func TestStatusTracker_Start(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)

	status := tracker.GetStatus()
	if !status.IsImporting {
		t.Error("Tracker should be importing after Start")
	}
	if status.OFLVersion != "v1.0.0" {
		t.Errorf("OFLVersion should be v1.0.0, got %s", status.OFLVersion)
	}
	if status.UsingBundledData {
		t.Error("UsingBundledData should be false")
	}
	if status.Phase != PhaseDownloading {
		t.Errorf("Phase should be DOWNLOADING, got %s", status.Phase)
	}
	if status.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
}

func TestStatusTracker_StartWithBundled(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("bundled", true)

	status := tracker.GetStatus()
	if !status.UsingBundledData {
		t.Error("UsingBundledData should be true")
	}
}

func TestStatusTracker_SetPhase(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)

	phases := []ImportPhase{
		PhaseExtracting,
		PhaseParsing,
		PhaseImporting,
	}

	for _, phase := range phases {
		tracker.SetPhase(phase)
		status := tracker.GetStatus()
		if status.Phase != phase {
			t.Errorf("Expected phase %s, got %s", phase, status.Phase)
		}
	}
}

func TestStatusTracker_SetTotalFixtures(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(100)

	status := tracker.GetStatus()
	if status.TotalFixtures != 100 {
		t.Errorf("TotalFixtures should be 100, got %d", status.TotalFixtures)
	}
}

func TestStatusTracker_SetCurrentFixture(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetCurrentFixture("TestMfg", "TestModel")

	status := tracker.GetStatus()
	if status.CurrentManufacturer != "TestMfg" {
		t.Errorf("CurrentManufacturer should be TestMfg, got %s", status.CurrentManufacturer)
	}
	if status.CurrentFixture != "TestModel" {
		t.Errorf("CurrentFixture should be TestModel, got %s", status.CurrentFixture)
	}
}

func TestStatusTracker_IncrementImported(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(10)
	tracker.SetPhase(PhaseImporting)

	tracker.IncrementImported()
	status := tracker.GetStatus()
	if status.ImportedCount != 1 {
		t.Errorf("ImportedCount should be 1, got %d", status.ImportedCount)
	}
}

func TestStatusTracker_IncrementFailed(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(10)
	tracker.SetPhase(PhaseImporting)

	tracker.IncrementFailed()
	status := tracker.GetStatus()
	if status.FailedCount != 1 {
		t.Errorf("FailedCount should be 1, got %d", status.FailedCount)
	}
}

func TestStatusTracker_IncrementSkipped(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(10)
	tracker.SetPhase(PhaseImporting)

	tracker.IncrementSkipped()
	status := tracker.GetStatus()
	if status.SkippedCount != 1 {
		t.Errorf("SkippedCount should be 1, got %d", status.SkippedCount)
	}
}

func TestStatusTracker_PercentComplete(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(100)
	tracker.SetPhase(PhaseImporting)

	// Process 50 fixtures
	for i := 0; i < 50; i++ {
		tracker.IncrementImported()
	}

	status := tracker.GetStatus()
	if status.PercentComplete < 49.0 || status.PercentComplete > 51.0 {
		t.Errorf("PercentComplete should be around 50, got %f", status.PercentComplete)
	}
}

func TestStatusTracker_Complete(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(10)
	tracker.SetPhase(PhaseImporting)

	for i := 0; i < 10; i++ {
		tracker.IncrementImported()
	}

	tracker.Complete()
	status := tracker.GetStatus()

	if status.IsImporting {
		t.Error("Tracker should not be importing after Complete")
	}
	if status.Phase != PhaseComplete {
		t.Errorf("Phase should be COMPLETE, got %s", status.Phase)
	}
	if status.PercentComplete != 100 {
		t.Errorf("PercentComplete should be 100, got %f", status.PercentComplete)
	}
	if status.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestStatusTracker_Fail(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetPhase(PhaseImporting)

	testErr := errors.New("test error")
	tracker.Fail(testErr)
	status := tracker.GetStatus()

	if status.IsImporting {
		t.Error("Tracker should not be importing after Fail")
	}
	if status.Phase != PhaseFailed {
		t.Errorf("Phase should be FAILED, got %s", status.Phase)
	}
	if status.ErrorMessage != "test error" {
		t.Errorf("ErrorMessage should be 'test error', got %s", status.ErrorMessage)
	}
}

func TestStatusTracker_FailNilError(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetPhase(PhaseImporting)

	tracker.Fail(nil)
	status := tracker.GetStatus()

	if status.Phase != PhaseFailed {
		t.Errorf("Phase should be FAILED, got %s", status.Phase)
	}
	if status.ErrorMessage != "" {
		t.Errorf("ErrorMessage should be empty for nil error, got %s", status.ErrorMessage)
	}
}

func TestStatusTracker_Cancel(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetPhase(PhaseImporting)

	tracker.Cancel()
	status := tracker.GetStatus()

	if status.IsImporting {
		t.Error("Tracker should not be importing after Cancel")
	}
	if status.Phase != PhaseCancelled {
		t.Errorf("Phase should be CANCELLED, got %s", status.Phase)
	}
}

func TestStatusTracker_Subscribe(t *testing.T) {
	tracker := NewStatusTracker()

	received := make(chan *ProgressStatus, 10)
	unsubscribe := tracker.Subscribe(func(status *ProgressStatus) {
		received <- status
	})
	defer unsubscribe()

	tracker.Start("v1.0.0", false)
	tracker.SetPhase(PhaseImporting)
	tracker.IncrementImported()
	tracker.Complete()

	// Allow time for callbacks
	time.Sleep(50 * time.Millisecond)

	count := len(received)
	if count < 3 {
		t.Errorf("Expected at least 3 status updates, got %d", count)
	}
}

func TestStatusTracker_Unsubscribe(t *testing.T) {
	tracker := NewStatusTracker()

	callCount := 0
	unsubscribe := tracker.Subscribe(func(status *ProgressStatus) {
		callCount++
	})

	tracker.Start("v1.0.0", false) // Should trigger callback
	unsubscribe()
	tracker.SetPhase(PhaseImporting) // Should NOT trigger callback

	time.Sleep(50 * time.Millisecond)

	if callCount > 1 {
		t.Errorf("Expected 1 callback after unsubscribe, got %d", callCount)
	}
}

func TestStatusTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(1000)
	tracker.SetPhase(PhaseImporting)

	var wg sync.WaitGroup

	// Concurrent increments
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.IncrementImported()
		}()
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tracker.GetStatus()
		}()
	}

	wg.Wait()

	status := tracker.GetStatus()
	if status.ImportedCount != 100 {
		t.Errorf("ImportedCount should be 100, got %d", status.ImportedCount)
	}
}

func TestStatusTracker_Reset(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(100)
	tracker.SetPhase(PhaseImporting)
	for i := 0; i < 50; i++ {
		tracker.IncrementImported()
	}
	tracker.Complete()

	// Reset the tracker
	tracker.Reset()
	status := tracker.GetStatus()

	if status.IsImporting {
		t.Error("IsImporting should be false after Reset")
	}
	if status.Phase != PhaseIdle {
		t.Errorf("Phase should be IDLE after Reset, got %s", status.Phase)
	}
}

func TestStatusTracker_StartResetsState(t *testing.T) {
	tracker := NewStatusTracker()
	tracker.Start("v1.0.0", false)
	tracker.SetTotalFixtures(100)
	for i := 0; i < 50; i++ {
		tracker.IncrementImported()
	}
	tracker.Complete()

	// Start a new import
	tracker.Start("v2.0.0", true)
	status := tracker.GetStatus()

	if status.OFLVersion != "v2.0.0" {
		t.Errorf("OFLVersion should be v2.0.0, got %s", status.OFLVersion)
	}
	if status.ImportedCount != 0 {
		t.Errorf("ImportedCount should be reset to 0, got %d", status.ImportedCount)
	}
	if status.TotalFixtures != 0 {
		t.Errorf("TotalFixtures should be reset to 0, got %d", status.TotalFixtures)
	}
}
