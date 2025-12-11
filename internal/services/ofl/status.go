package ofl

import (
	"sync"
	"time"
)

// ImportPhase represents the current phase of an OFL import
type ImportPhase string

const (
	PhaseIdle        ImportPhase = "IDLE"
	PhaseDownloading ImportPhase = "DOWNLOADING"
	PhaseExtracting  ImportPhase = "EXTRACTING"
	PhaseParsing     ImportPhase = "PARSING"
	PhaseImporting   ImportPhase = "IMPORTING"
	PhaseComplete    ImportPhase = "COMPLETE"
	PhaseFailed      ImportPhase = "FAILED"
	PhaseCancelled   ImportPhase = "CANCELLED"
)

// ProgressStatus represents the real-time status of an OFL import
type ProgressStatus struct {
	IsImporting               bool        `json:"isImporting"`
	Phase                     ImportPhase `json:"phase"`
	TotalFixtures             int         `json:"totalFixtures"`
	ImportedCount             int         `json:"importedCount"`
	FailedCount               int         `json:"failedCount"`
	SkippedCount              int         `json:"skippedCount"`
	PercentComplete           float64     `json:"percentComplete"`
	CurrentFixture            string      `json:"currentFixture,omitempty"`
	CurrentManufacturer       string      `json:"currentManufacturer,omitempty"`
	EstimatedSecondsRemaining *int        `json:"estimatedSecondsRemaining,omitempty"`
	ErrorMessage              string      `json:"errorMessage,omitempty"`
	StartedAt                 *time.Time  `json:"startedAt,omitempty"`
	CompletedAt               *time.Time  `json:"completedAt,omitempty"`
	OFLVersion                string      `json:"oflVersion,omitempty"`
	UsingBundledData          bool        `json:"usingBundledData"`
}

// StatusCallback is a function called when the import status changes
type StatusCallback func(status *ProgressStatus)

// StatusTracker tracks the progress of an OFL import with thread safety
type StatusTracker struct {
	mu        sync.RWMutex
	status    ProgressStatus
	callbacks []StatusCallback
	startTime time.Time
}

// NewStatusTracker creates a new status tracker
func NewStatusTracker() *StatusTracker {
	return &StatusTracker{
		status: ProgressStatus{
			IsImporting: false,
			Phase:       PhaseIdle,
		},
		callbacks: make([]StatusCallback, 0),
	}
}

// Subscribe adds a callback for status updates. Returns an unsubscribe function.
func (t *StatusTracker) Subscribe(callback StatusCallback) func() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.callbacks = append(t.callbacks, callback)
	idx := len(t.callbacks) - 1

	// Return unsubscribe function
	return func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		// Mark as nil instead of removing to preserve indices
		if idx < len(t.callbacks) {
			t.callbacks[idx] = nil
		}
	}
}

// GetStatus returns a copy of the current status
func (t *StatusTracker) GetStatus() ProgressStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

// notifyCallbacks sends the current status to all subscribers
func (t *StatusTracker) notifyCallbacks() {
	// Make a copy of the status and callbacks to avoid holding the lock during callbacks
	t.mu.RLock()
	statusCopy := t.status
	callbacksCopy := make([]StatusCallback, len(t.callbacks))
	copy(callbacksCopy, t.callbacks)
	t.mu.RUnlock()

	for _, cb := range callbacksCopy {
		if cb != nil {
			cb(&statusCopy)
		}
	}
}

// Start begins tracking a new import
func (t *StatusTracker) Start(oflVersion string, usingBundled bool) {
	t.mu.Lock()
	now := time.Now()
	t.startTime = now
	t.status = ProgressStatus{
		IsImporting:      true,
		Phase:            PhaseDownloading,
		TotalFixtures:    0,
		ImportedCount:    0,
		FailedCount:      0,
		SkippedCount:     0,
		PercentComplete:  0,
		StartedAt:        &now,
		OFLVersion:       oflVersion,
		UsingBundledData: usingBundled,
	}
	t.mu.Unlock()
	t.notifyCallbacks()
}

// SetPhase updates the current phase
func (t *StatusTracker) SetPhase(phase ImportPhase) {
	t.mu.Lock()
	t.status.Phase = phase
	t.mu.Unlock()
	t.notifyCallbacks()
}

// SetTotalFixtures sets the total number of fixtures to import
func (t *StatusTracker) SetTotalFixtures(total int) {
	t.mu.Lock()
	t.status.TotalFixtures = total
	t.updatePercentComplete()
	t.mu.Unlock()
	t.notifyCallbacks()
}

// SetCurrentFixture updates the currently processing fixture
func (t *StatusTracker) SetCurrentFixture(manufacturer, model string) {
	t.mu.Lock()
	t.status.CurrentManufacturer = manufacturer
	t.status.CurrentFixture = model
	t.mu.Unlock()
	t.notifyCallbacks()
}

// IncrementImported increments the imported count
func (t *StatusTracker) IncrementImported() {
	t.mu.Lock()
	t.status.ImportedCount++
	t.updatePercentComplete()
	t.updateEstimatedTime()
	t.mu.Unlock()
	t.notifyCallbacks()
}

// IncrementFailed increments the failed count
func (t *StatusTracker) IncrementFailed() {
	t.mu.Lock()
	t.status.FailedCount++
	t.updatePercentComplete()
	t.updateEstimatedTime()
	t.mu.Unlock()
	t.notifyCallbacks()
}

// IncrementSkipped increments the skipped count
func (t *StatusTracker) IncrementSkipped() {
	t.mu.Lock()
	t.status.SkippedCount++
	t.updatePercentComplete()
	t.updateEstimatedTime()
	t.mu.Unlock()
	t.notifyCallbacks()
}

// updatePercentComplete calculates the percentage (called with lock held)
func (t *StatusTracker) updatePercentComplete() {
	if t.status.TotalFixtures > 0 {
		processed := t.status.ImportedCount + t.status.FailedCount + t.status.SkippedCount
		t.status.PercentComplete = float64(processed) / float64(t.status.TotalFixtures) * 100
	}
}

// updateEstimatedTime calculates ETA (called with lock held)
func (t *StatusTracker) updateEstimatedTime() {
	processed := t.status.ImportedCount + t.status.FailedCount + t.status.SkippedCount
	remaining := t.status.TotalFixtures - processed

	if processed > 0 && remaining > 0 {
		elapsed := time.Since(t.startTime).Seconds()
		avgPerFixture := elapsed / float64(processed)
		estimatedRemaining := int(avgPerFixture * float64(remaining))
		t.status.EstimatedSecondsRemaining = &estimatedRemaining
	} else {
		t.status.EstimatedSecondsRemaining = nil
	}
}

// Complete marks the import as complete
func (t *StatusTracker) Complete() {
	t.mu.Lock()
	now := time.Now()
	t.status.IsImporting = false
	t.status.Phase = PhaseComplete
	t.status.CompletedAt = &now
	t.status.PercentComplete = 100
	t.status.EstimatedSecondsRemaining = nil
	t.status.CurrentFixture = ""
	t.status.CurrentManufacturer = ""
	t.mu.Unlock()
	t.notifyCallbacks()
}

// Fail marks the import as failed with an error message
func (t *StatusTracker) Fail(err error) {
	t.mu.Lock()
	now := time.Now()
	t.status.IsImporting = false
	t.status.Phase = PhaseFailed
	t.status.CompletedAt = &now
	t.status.EstimatedSecondsRemaining = nil
	if err != nil {
		t.status.ErrorMessage = err.Error()
	}
	t.mu.Unlock()
	t.notifyCallbacks()
}

// Cancel marks the import as cancelled
func (t *StatusTracker) Cancel() {
	t.mu.Lock()
	now := time.Now()
	t.status.IsImporting = false
	t.status.Phase = PhaseCancelled
	t.status.CompletedAt = &now
	t.status.EstimatedSecondsRemaining = nil
	t.mu.Unlock()
	t.notifyCallbacks()
}

// Reset resets the tracker to idle state
func (t *StatusTracker) Reset() {
	t.mu.Lock()
	t.status = ProgressStatus{
		IsImporting: false,
		Phase:       PhaseIdle,
	}
	t.mu.Unlock()
	t.notifyCallbacks()
}
