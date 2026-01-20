package resolvers

// This file contains resolver implementations for Effect-related types.
// The methods in this file are called from schema.resolvers.go which contains
// the auto-generated resolver stubs.

import (
	"context"
	"fmt"
	"time"

	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
	"github.com/bbernstein/lacylights-go/internal/services/undo"
)

// ============================================================================
// Query Resolvers
// ============================================================================

// ResolveEffect returns a single effect by ID.
func (r *Resolver) ResolveEffect(ctx context.Context, id string) (*models.Effect, error) {
	effect, err := r.EffectRepo.FindWithFixtures(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return nil, fmt.Errorf("effect not found: %s", id)
	}
	return effect, nil
}

// ResolveEffects returns all effects in a project.
func (r *Resolver) ResolveEffects(ctx context.Context, projectID string) ([]*models.Effect, error) {
	effects, err := r.EffectRepo.FindByProjectID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to find effects: %w", err)
	}
	return effects, nil
}

// ResolveModulatorStatus returns the current status of the modulator engine.
func (r *Resolver) ResolveModulatorStatus(ctx context.Context) (*generated.ModulatorStatus, error) {
	if r.FadeEngine == nil {
		return &generated.ModulatorStatus{
			IsRunning:           false,
			UpdateRateHz:        60,
			ActiveEffectCount:   0,
			ActiveEffects:       []*generated.ActiveEffectStatus{},
			IsBlackoutActive:    false,
			BlackoutIntensity:   0,
			GrandMasterValue:    1.0,
			HasActiveTransition: false,
			TransitionProgress:  0,
		}, nil
	}

	activeEffects := r.FadeEngine.GetActiveEffects()
	activeEffectStatuses := make([]*generated.ActiveEffectStatus, 0, len(activeEffects))

	hasActiveTransition := false
	var transitionProgress float64

	for _, active := range activeEffects {
		effectType := generated.EffectType(active.Effect.EffectType.String())

		status := &generated.ActiveEffectStatus{
			EffectID:   active.Effect.ID,
			EffectName: active.Effect.Name,
			EffectType: effectType,
			Intensity:  active.GetCurrentIntensity(),
			Phase:      active.Phase,
			IsComplete: active.IsComplete(),
			StartTime:  active.StartTime.Format(time.RFC3339),
		}
		activeEffectStatuses = append(activeEffectStatuses, status)

		// Check for active transitions (crossfades)
		if active.Effect.EffectType == modulator.EffectTypeCrossfade && !active.IsComplete() {
			hasActiveTransition = true
			transitionProgress = active.GetProgress() * 100
		}
	}

	return &generated.ModulatorStatus{
		IsRunning:           r.FadeEngine.IsRunning(),
		UpdateRateHz:        r.FadeEngine.GetUpdateRateHz(),
		ActiveEffectCount:   r.FadeEngine.GetActiveEffectCount(),
		ActiveEffects:       activeEffectStatuses,
		IsBlackoutActive:    r.FadeEngine.IsBlackoutActive(),
		BlackoutIntensity:   r.FadeEngine.GetBlackoutIntensity(),
		GrandMasterValue:    r.FadeEngine.GetGrandMasterValue(),
		HasActiveTransition: hasActiveTransition,
		TransitionProgress:  transitionProgress,
	}, nil
}

// ============================================================================
// Mutation Resolvers
// ============================================================================

// ResolveCreateEffect creates a new effect.
func (r *Resolver) ResolveCreateEffect(ctx context.Context, input generated.CreateEffectInput) (*models.Effect, error) {
	effect := &models.Effect{
		Name:            input.Name,
		ProjectID:       input.ProjectID,
		EffectType:      string(input.EffectType),
		PriorityBand:    "USER",
		PrioritySub:     50,
		CompositionMode: "OVERRIDE",
		OnCueChange:     "FADE_OUT",
		Frequency:       1.0,
		Amplitude:       100.0,
		Offset:          50.0,
		PhaseOffset:     0.0,
	}

	if input.Description.IsSet() && input.Description.Value() != nil {
		effect.Description = input.Description.Value()
	}

	if input.PriorityBand.IsSet() && input.PriorityBand.Value() != nil {
		effect.PriorityBand = string(*input.PriorityBand.Value())
	}
	if input.PrioritySub.IsSet() && input.PrioritySub.Value() != nil {
		effect.PrioritySub = *input.PrioritySub.Value()
	}
	if input.CompositionMode.IsSet() && input.CompositionMode.Value() != nil {
		effect.CompositionMode = string(*input.CompositionMode.Value())
	}
	if input.OnCueChange.IsSet() && input.OnCueChange.Value() != nil {
		effect.OnCueChange = string(*input.OnCueChange.Value())
	}
	if input.FadeDuration.IsSet() {
		effect.FadeDuration = input.FadeDuration.Value()
	}
	if input.Waveform.IsSet() && input.Waveform.Value() != nil {
		waveform := string(*input.Waveform.Value())
		effect.Waveform = &waveform
	}
	if input.Frequency.IsSet() && input.Frequency.Value() != nil {
		effect.Frequency = *input.Frequency.Value()
	}
	if input.Amplitude.IsSet() && input.Amplitude.Value() != nil {
		effect.Amplitude = *input.Amplitude.Value()
	}
	if input.Offset.IsSet() && input.Offset.Value() != nil {
		effect.Offset = *input.Offset.Value()
	}
	if input.PhaseOffset.IsSet() && input.PhaseOffset.Value() != nil {
		effect.PhaseOffset = *input.PhaseOffset.Value()
	}
	if input.MasterValue.IsSet() {
		effect.MasterValue = input.MasterValue.Value()
	}

	if err := r.EffectRepo.Create(ctx, effect); err != nil {
		return nil, fmt.Errorf("failed to create effect: %w", err)
	}

	// Record undo operation
	if newState, err := r.UndoService.CaptureEffectState(ctx, effect.ID); err == nil {
		_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeCreate, undo.EntityTypeEffect, effect.ID,
			fmt.Sprintf("Create effect '%s'", effect.Name), nil, newState, nil)
	}

	return effect, nil
}

// ResolveUpdateEffect updates an existing effect.
func (r *Resolver) ResolveUpdateEffect(ctx context.Context, id string, input generated.UpdateEffectInput) (*models.Effect, error) {
	effect, err := r.EffectRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return nil, fmt.Errorf("effect not found: %s", id)
	}

	// Capture state before update
	prevState, _ := r.UndoService.CaptureEffectState(ctx, id)

	if input.Name.IsSet() && input.Name.Value() != nil {
		effect.Name = *input.Name.Value()
	}
	if input.Description.IsSet() {
		effect.Description = input.Description.Value()
	}
	if input.EffectType.IsSet() && input.EffectType.Value() != nil {
		effect.EffectType = string(*input.EffectType.Value())
	}
	if input.PriorityBand.IsSet() && input.PriorityBand.Value() != nil {
		effect.PriorityBand = string(*input.PriorityBand.Value())
	}
	if input.PrioritySub.IsSet() && input.PrioritySub.Value() != nil {
		effect.PrioritySub = *input.PrioritySub.Value()
	}
	if input.CompositionMode.IsSet() && input.CompositionMode.Value() != nil {
		effect.CompositionMode = string(*input.CompositionMode.Value())
	}
	if input.OnCueChange.IsSet() && input.OnCueChange.Value() != nil {
		effect.OnCueChange = string(*input.OnCueChange.Value())
	}
	if input.FadeDuration.IsSet() {
		effect.FadeDuration = input.FadeDuration.Value()
	}
	if input.Waveform.IsSet() && input.Waveform.Value() != nil {
		waveform := string(*input.Waveform.Value())
		effect.Waveform = &waveform
	}
	if input.Frequency.IsSet() && input.Frequency.Value() != nil {
		effect.Frequency = *input.Frequency.Value()
	}
	if input.Amplitude.IsSet() && input.Amplitude.Value() != nil {
		effect.Amplitude = *input.Amplitude.Value()
	}
	if input.Offset.IsSet() && input.Offset.Value() != nil {
		effect.Offset = *input.Offset.Value()
	}
	if input.PhaseOffset.IsSet() && input.PhaseOffset.Value() != nil {
		effect.PhaseOffset = *input.PhaseOffset.Value()
	}
	if input.MasterValue.IsSet() {
		effect.MasterValue = input.MasterValue.Value()
	}

	if err := r.EffectRepo.Update(ctx, effect); err != nil {
		return nil, fmt.Errorf("failed to update effect: %w", err)
	}

	// Record undo operation
	if newState, err := r.UndoService.CaptureEffectState(ctx, id); err == nil && prevState != nil {
		_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, id,
			fmt.Sprintf("Update effect '%s'", effect.Name), prevState, newState, nil)
	}

	return effect, nil
}

// ResolveDeleteEffect deletes an effect.
func (r *Resolver) ResolveDeleteEffect(ctx context.Context, id string) (bool, error) {
	// Capture state before delete
	prevState, _ := r.UndoService.CaptureEffectState(ctx, id)
	var projectID string
	var effectName string
	if prevState != nil && prevState.Effect != nil {
		projectID = prevState.Effect.ProjectID
		effectName = prevState.Effect.Name
	}

	if err := r.EffectRepo.Delete(ctx, id); err != nil {
		return false, fmt.Errorf("failed to delete effect: %w", err)
	}

	// Record undo operation
	if prevState != nil && projectID != "" {
		_ = r.UndoService.RecordOperation(ctx, projectID, undo.OperationTypeDelete, undo.EntityTypeEffect, id,
			fmt.Sprintf("Delete effect '%s'", effectName), prevState, nil, nil)
	}

	return true, nil
}

// ResolveAddFixtureToEffect adds a fixture to an effect.
func (r *Resolver) ResolveAddFixtureToEffect(ctx context.Context, input generated.AddFixtureToEffectInput) (*models.EffectFixture, error) {
	// Verify effect exists
	effect, err := r.EffectRepo.FindByID(ctx, input.EffectID)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return nil, fmt.Errorf("effect not found: %s", input.EffectID)
	}

	// Verify fixture exists
	fixture, err := r.FixtureRepo.FindByID(ctx, input.FixtureID)
	if err != nil {
		return nil, fmt.Errorf("failed to find fixture: %w", err)
	}
	if fixture == nil {
		return nil, fmt.Errorf("fixture not found: %s", input.FixtureID)
	}

	// Capture state before modification
	prevState, _ := r.UndoService.CaptureEffectState(ctx, input.EffectID)

	var phaseOffset *float64
	var amplitudeScale *float64

	if input.PhaseOffset.IsSet() {
		phaseOffset = input.PhaseOffset.Value()
	}
	if input.AmplitudeScale.IsSet() {
		amplitudeScale = input.AmplitudeScale.Value()
	}

	if err := r.EffectRepo.AddFixtureToEffect(ctx, input.EffectID, input.FixtureID, phaseOffset, amplitudeScale); err != nil {
		return nil, fmt.Errorf("failed to add fixture to effect: %w", err)
	}

	// Return the created effect fixture
	effectFixture, err := r.EffectRepo.GetEffectFixture(ctx, input.EffectID, input.FixtureID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effect fixture: %w", err)
	}

	// Record undo operation
	if newState, err := r.UndoService.CaptureEffectState(ctx, input.EffectID); err == nil && prevState != nil {
		_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, input.EffectID,
			fmt.Sprintf("Add fixture to effect '%s'", effect.Name), prevState, newState, nil)
	}

	return effectFixture, nil
}

// ResolveRemoveFixtureFromEffect removes a fixture from an effect.
func (r *Resolver) ResolveRemoveFixtureFromEffect(ctx context.Context, effectID, fixtureID string) (bool, error) {
	// Get effect for project ID
	effect, err := r.EffectRepo.FindByID(ctx, effectID)
	if err != nil {
		return false, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return false, fmt.Errorf("effect not found: %s", effectID)
	}

	// Capture state before modification
	prevState, _ := r.UndoService.CaptureEffectState(ctx, effectID)

	if err := r.EffectRepo.RemoveFixtureFromEffect(ctx, effectID, fixtureID); err != nil {
		return false, fmt.Errorf("failed to remove fixture from effect: %w", err)
	}

	// Record undo operation
	if newState, err := r.UndoService.CaptureEffectState(ctx, effectID); err == nil && prevState != nil {
		_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effectID,
			fmt.Sprintf("Remove fixture from effect '%s'", effect.Name), prevState, newState, nil)
	}

	return true, nil
}

// ResolveAddEffectToCue adds an effect to a cue.
func (r *Resolver) ResolveAddEffectToCue(ctx context.Context, input generated.AddEffectToCueInput) (*models.CueEffect, error) {
	// Verify cue exists
	cue, err := r.CueRepo.FindByID(ctx, input.CueID)
	if err != nil {
		return nil, fmt.Errorf("failed to find cue: %w", err)
	}
	if cue == nil {
		return nil, fmt.Errorf("cue not found: %s", input.CueID)
	}

	// Get the cue list to find the project ID
	cueList, err := r.CueListRepo.FindByID(ctx, cue.CueListID)
	if err != nil {
		return nil, fmt.Errorf("failed to find cue list: %w", err)
	}
	projectID := ""
	if cueList != nil {
		projectID = cueList.ProjectID
	}

	// Verify effect exists
	effect, err := r.EffectRepo.FindByID(ctx, input.EffectID)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return nil, fmt.Errorf("effect not found: %s", input.EffectID)
	}

	intensity := 100.0
	speed := 1.0

	if input.Intensity.IsSet() && input.Intensity.Value() != nil {
		intensity = *input.Intensity.Value()
	}
	if input.Speed.IsSet() && input.Speed.Value() != nil {
		speed = *input.Speed.Value()
	}

	if err := r.EffectRepo.AddEffectToCue(ctx, input.CueID, input.EffectID, intensity, speed); err != nil {
		return nil, fmt.Errorf("failed to add effect to cue: %w", err)
	}

	// Get the created cue effect
	cueEffect, err := r.EffectRepo.GetCueEffect(ctx, input.CueID, input.EffectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cue effect: %w", err)
	}

	// Handle optional onCueChange override
	if input.OnCueChange.IsSet() && input.OnCueChange.Value() != nil && cueEffect != nil {
		onCueChange := string(*input.OnCueChange.Value())
		cueEffect.OnCueChange = &onCueChange
		if err := r.EffectRepo.UpdateCueEffect(ctx, cueEffect); err != nil {
			return nil, fmt.Errorf("failed to update cue effect: %w", err)
		}
	}

	// Record undo operation (capture state after creation)
	if projectID != "" && cueEffect != nil {
		if newState, err := r.UndoService.CaptureCueEffectState(ctx, input.CueID, input.EffectID); err == nil {
			entityID := fmt.Sprintf("%s:%s", input.CueID, input.EffectID)
			_ = r.UndoService.RecordOperation(ctx, projectID, undo.OperationTypeCreate, undo.EntityTypeCueEffect, entityID,
				fmt.Sprintf("Add effect '%s' to cue", effect.Name), nil, newState, nil)
		}
	}

	return cueEffect, nil
}

// ResolveRemoveEffectFromCue removes an effect from a cue.
func (r *Resolver) ResolveRemoveEffectFromCue(ctx context.Context, cueID, effectID string) (bool, error) {
	// Capture state before deletion for undo
	var prevState *undo.CueEffectSnapshot
	var projectID string
	var effectName string

	if cueEffect, err := r.UndoService.CaptureCueEffectState(ctx, cueID, effectID); err == nil && cueEffect != nil {
		prevState = cueEffect
		projectID = cueEffect.ProjectID
	}

	// Get effect name for description
	if effect, err := r.EffectRepo.FindByID(ctx, effectID); err == nil && effect != nil {
		effectName = effect.Name
	}

	if err := r.EffectRepo.RemoveEffectFromCue(ctx, cueID, effectID); err != nil {
		return false, fmt.Errorf("failed to remove effect from cue: %w", err)
	}

	// Record undo operation
	if projectID != "" && prevState != nil {
		entityID := fmt.Sprintf("%s:%s", cueID, effectID)
		_ = r.UndoService.RecordOperation(ctx, projectID, undo.OperationTypeDelete, undo.EntityTypeCueEffect, entityID,
			fmt.Sprintf("Remove effect '%s' from cue", effectName), prevState, nil, nil)
	}

	return true, nil
}

// ResolveActivateEffect activates an effect with optional fade time.
func (r *Resolver) ResolveActivateEffect(ctx context.Context, effectID string, fadeTime *float64) (bool, error) {
	if r.FadeEngine == nil {
		return false, fmt.Errorf("modulator engine not available")
	}

	// Load the effect from database
	effect, err := r.EffectRepo.FindWithFixtures(ctx, effectID)
	if err != nil {
		return false, fmt.Errorf("failed to find effect: %w", err)
	}
	if effect == nil {
		return false, fmt.Errorf("effect not found: %s", effectID)
	}

	// Convert database model to modulator Effect
	modulatorEffect := convertToModulatorEffect(effect)

	// Add to the engine
	activeEffect := r.FadeEngine.AddEffect(modulatorEffect)

	// Apply fade-in if specified
	if fadeTime != nil && *fadeTime > 0 {
		activeEffect.Intensity = 0
		activeEffect.FadeIntensityTo(100, time.Duration(*fadeTime*float64(time.Second)), modulator.EasingLinear)
	}

	return true, nil
}

// ResolveStopEffect stops an active effect with optional fade time.
func (r *Resolver) ResolveStopEffect(ctx context.Context, effectID string, fadeTime *float64) (bool, error) {
	if r.FadeEngine == nil {
		return false, fmt.Errorf("modulator engine not available")
	}

	activeEffects := r.FadeEngine.GetActiveEffects()
	for _, active := range activeEffects {
		if active.Effect.ID == effectID {
			if fadeTime != nil && *fadeTime > 0 {
				// Fade out the effect
				active.FadeIntensityTo(0, time.Duration(*fadeTime*float64(time.Second)), modulator.EasingLinear)
			} else {
				// Immediately remove the effect
				r.FadeEngine.RemoveEffect(effectID)
			}
			return true, nil
		}
	}

	// Effect not found in active effects
	return false, nil
}

// ResolveActivateBlackout activates the system blackout.
func (r *Resolver) ResolveActivateBlackout(ctx context.Context, fadeTime *float64) (bool, error) {
	if r.FadeEngine == nil {
		return false, fmt.Errorf("modulator engine not available")
	}

	ft := 0.0
	if fadeTime != nil {
		ft = *fadeTime
	}
	r.FadeEngine.ActivateBlackout(ft)
	return true, nil
}

// ResolveReleaseBlackout releases the system blackout.
func (r *Resolver) ResolveReleaseBlackout(ctx context.Context, fadeTime *float64) (bool, error) {
	if r.FadeEngine == nil {
		return false, fmt.Errorf("modulator engine not available")
	}

	ft := 0.0
	if fadeTime != nil {
		ft = *fadeTime
	}
	r.FadeEngine.ReleaseBlackout(ft)
	return true, nil
}

// ResolveSetGrandMaster sets the grand master level.
func (r *Resolver) ResolveSetGrandMaster(ctx context.Context, value float64) (bool, error) {
	if r.FadeEngine == nil {
		return false, fmt.Errorf("modulator engine not available")
	}

	r.FadeEngine.SetGrandMaster(value)
	return true, nil
}

// ============================================================================
// Effect Fixture & Channel Management Resolvers
// ============================================================================

// ResolveUpdateEffectFixture updates an effect fixture's settings (phase offset, amplitude scale, effect order).
func (r *Resolver) ResolveUpdateEffectFixture(ctx context.Context, id string, input generated.UpdateEffectFixtureInput) (*models.EffectFixture, error) {
	effectFixture, err := r.EffectRepo.FindEffectFixtureByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect fixture: %w", err)
	}
	if effectFixture == nil {
		return nil, fmt.Errorf("effect fixture not found: %s", id)
	}

	// Get parent effect for undo recording
	effect, _ := r.EffectRepo.FindByID(ctx, effectFixture.EffectID)

	// Capture state before modification
	prevState, _ := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID)

	if input.PhaseOffset.IsSet() {
		effectFixture.PhaseOffset = input.PhaseOffset.Value()
	}
	if input.AmplitudeScale.IsSet() {
		effectFixture.AmplitudeScale = input.AmplitudeScale.Value()
	}
	if input.EffectOrder.IsSet() {
		effectFixture.EffectOrder = input.EffectOrder.Value()
	}

	if err := r.EffectRepo.UpdateEffectFixture(ctx, effectFixture); err != nil {
		return nil, fmt.Errorf("failed to update effect fixture: %w", err)
	}

	// Record undo operation
	if effect != nil && prevState != nil {
		if newState, err := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID); err == nil {
			_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effectFixture.EffectID,
				fmt.Sprintf("Update fixture settings in effect '%s'", effect.Name), prevState, newState, nil)
		}
	}

	// Re-fetch to get updated data with preloaded relationships
	return r.EffectRepo.FindEffectFixtureByID(ctx, id)
}

// ResolveAddChannelToEffectFixture adds a channel to an effect fixture.
func (r *Resolver) ResolveAddChannelToEffectFixture(ctx context.Context, effectFixtureID string, input generated.EffectChannelInput) (*models.EffectChannel, error) {
	// Verify effect fixture exists
	effectFixture, err := r.EffectRepo.FindEffectFixtureByID(ctx, effectFixtureID)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect fixture: %w", err)
	}
	if effectFixture == nil {
		return nil, fmt.Errorf("effect fixture not found: %s", effectFixtureID)
	}

	// Get parent effect for undo recording
	effect, _ := r.EffectRepo.FindByID(ctx, effectFixture.EffectID)

	// Capture state before modification
	prevState, _ := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID)

	var channelOffset *int
	var channelType *string
	var amplitudeScale *float64
	var frequencyScale *float64

	if input.ChannelOffset.IsSet() {
		channelOffset = input.ChannelOffset.Value()
	}
	if input.ChannelType.IsSet() && input.ChannelType.Value() != nil {
		ct := string(*input.ChannelType.Value())
		channelType = &ct
	}
	if input.AmplitudeScale.IsSet() {
		amplitudeScale = input.AmplitudeScale.Value()
	}
	if input.FrequencyScale.IsSet() {
		frequencyScale = input.FrequencyScale.Value()
	}

	if err := r.EffectRepo.AddChannelToEffectFixtureWithScales(ctx, effectFixtureID, channelOffset, channelType, amplitudeScale, frequencyScale); err != nil {
		return nil, fmt.Errorf("failed to add channel to effect fixture: %w", err)
	}

	// Get the channels to find the newly created one
	channels, err := r.EffectRepo.GetEffectChannels(ctx, effectFixtureID)
	if err != nil {
		return nil, fmt.Errorf("failed to get effect channels: %w", err)
	}

	// Record undo operation
	if effect != nil && prevState != nil {
		if newState, err := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID); err == nil {
			_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effectFixture.EffectID,
				fmt.Sprintf("Add channel to effect '%s'", effect.Name), prevState, newState, nil)
		}
	}

	// Return the last channel (the one we just created)
	if len(channels) > 0 {
		return &channels[len(channels)-1], nil
	}

	return nil, fmt.Errorf("channel was created but not found")
}

// ResolveUpdateEffectChannel updates an effect channel's settings.
func (r *Resolver) ResolveUpdateEffectChannel(ctx context.Context, id string, input generated.EffectChannelInput) (*models.EffectChannel, error) {
	channel, err := r.EffectRepo.FindEffectChannelByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find effect channel: %w", err)
	}
	if channel == nil {
		return nil, fmt.Errorf("effect channel not found: %s", id)
	}

	// Get parent effect fixture and effect for undo recording
	effectFixture, _ := r.EffectRepo.FindEffectFixtureByID(ctx, channel.EffectFixtureID)
	var effect *models.Effect
	var prevState *undo.EffectSnapshot
	if effectFixture != nil {
		effect, _ = r.EffectRepo.FindByID(ctx, effectFixture.EffectID)
		prevState, _ = r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID)
	}

	if input.ChannelOffset.IsSet() {
		channel.ChannelOffset = input.ChannelOffset.Value()
	}
	if input.ChannelType.IsSet() && input.ChannelType.Value() != nil {
		ct := string(*input.ChannelType.Value())
		channel.ChannelType = &ct
	}
	if input.AmplitudeScale.IsSet() {
		channel.AmplitudeScale = input.AmplitudeScale.Value()
	}
	if input.FrequencyScale.IsSet() {
		channel.FrequencyScale = input.FrequencyScale.Value()
	}
	if input.MinValue.IsSet() {
		channel.MinValue = input.MinValue.Value()
	}
	if input.MaxValue.IsSet() {
		channel.MaxValue = input.MaxValue.Value()
	}

	if err := r.EffectRepo.UpdateEffectChannel(ctx, channel); err != nil {
		return nil, fmt.Errorf("failed to update effect channel: %w", err)
	}

	// Record undo operation
	if effect != nil && prevState != nil && effectFixture != nil {
		if newState, err := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID); err == nil {
			_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effectFixture.EffectID,
				fmt.Sprintf("Update channel in effect '%s'", effect.Name), prevState, newState, nil)
		}
	}

	return channel, nil
}

// ResolveRemoveChannelFromEffectFixture removes a channel from an effect fixture.
func (r *Resolver) ResolveRemoveChannelFromEffectFixture(ctx context.Context, id string) (bool, error) {
	// Verify channel exists
	channel, err := r.EffectRepo.FindEffectChannelByID(ctx, id)
	if err != nil {
		return false, fmt.Errorf("failed to find effect channel: %w", err)
	}
	if channel == nil {
		return false, fmt.Errorf("effect channel not found: %s", id)
	}

	// Get parent effect fixture and effect for undo recording
	effectFixture, _ := r.EffectRepo.FindEffectFixtureByID(ctx, channel.EffectFixtureID)
	var effect *models.Effect
	var prevState *undo.EffectSnapshot
	if effectFixture != nil {
		effect, _ = r.EffectRepo.FindByID(ctx, effectFixture.EffectID)
		prevState, _ = r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID)
	}

	if err := r.EffectRepo.DeleteEffectChannel(ctx, id); err != nil {
		return false, fmt.Errorf("failed to delete effect channel: %w", err)
	}

	// Record undo operation
	if effect != nil && prevState != nil && effectFixture != nil {
		if newState, err := r.UndoService.CaptureEffectState(ctx, effectFixture.EffectID); err == nil {
			_ = r.UndoService.RecordOperation(ctx, effect.ProjectID, undo.OperationTypeUpdate, undo.EntityTypeEffect, effectFixture.EffectID,
				fmt.Sprintf("Remove channel from effect '%s'", effect.Name), prevState, newState, nil)
		}
	}

	return true, nil
}

// ============================================================================
// Type Resolvers - Effect
// ============================================================================

// ResolveEffectType converts the string effect type to the enum.
func ResolveEffectType(obj *models.Effect) (generated.EffectType, error) {
	return generated.EffectType(obj.EffectType), nil
}

// ResolvePriorityBand converts the string priority band to the enum.
func ResolvePriorityBand(obj *models.Effect) (generated.PriorityBand, error) {
	return generated.PriorityBand(obj.PriorityBand), nil
}

// ResolveCompositionMode converts the string composition mode to the enum.
func ResolveCompositionMode(obj *models.Effect) (generated.CompositionMode, error) {
	return generated.CompositionMode(obj.CompositionMode), nil
}

// ResolveOnCueChangeEffect converts the string on cue change behavior to the enum.
func ResolveOnCueChangeEffect(obj *models.Effect) (generated.TransitionBehavior, error) {
	return generated.TransitionBehavior(obj.OnCueChange), nil
}

// ResolveWaveform converts the waveform string to the enum.
func ResolveWaveform(obj *models.Effect) (*generated.WaveformType, error) {
	if obj.Waveform == nil {
		return nil, nil
	}
	wt := generated.WaveformType(*obj.Waveform)
	return &wt, nil
}

// ============================================================================
// Type Resolvers - CueEffect
// ============================================================================

// ResolveOnCueChangeCueEffect converts the cue effect's on cue change behavior to the enum.
func ResolveOnCueChangeCueEffect(obj *models.CueEffect) (*generated.TransitionBehavior, error) {
	if obj.OnCueChange == nil {
		return nil, nil
	}
	tb := generated.TransitionBehavior(*obj.OnCueChange)
	return &tb, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// convertToModulatorEffect converts a database Effect model to a modulator Effect.
func convertToModulatorEffect(dbEffect *models.Effect) *modulator.Effect {
	effect := &modulator.Effect{
		ID:          dbEffect.ID,
		Name:        dbEffect.Name,
		ProjectID:   dbEffect.ProjectID,
		EffectType:  modulator.ParseEffectType(dbEffect.EffectType),
		Priority:    parseEffectPriority(dbEffect.PriorityBand, dbEffect.PrioritySub),
		CompositionMode: parseCompositionMode(dbEffect.CompositionMode),
		OnCueChange: parseTransitionBehavior(dbEffect.OnCueChange),
		Frequency:   dbEffect.Frequency,
		Amplitude:   dbEffect.Amplitude,
		Offset:      dbEffect.Offset,
		PhaseOffset: dbEffect.PhaseOffset,
	}

	if dbEffect.Description != nil {
		effect.Description = *dbEffect.Description
	}
	if dbEffect.FadeDuration != nil {
		effect.FadeDuration = dbEffect.FadeDuration
	}
	if dbEffect.Waveform != nil {
		effect.Waveform = modulator.ParseWaveformType(*dbEffect.Waveform)
	}
	if dbEffect.MasterValue != nil {
		effect.MasterValue = *dbEffect.MasterValue
	}

	// Convert fixtures to target channels
	for _, fixture := range dbEffect.Fixtures {
		if fixture.Fixture != nil {
			for _, channel := range fixture.Channels {
				effectChannel := modulator.EffectChannel{
					Universe: fixture.Fixture.Universe,
				}
				if channel.ChannelOffset != nil {
					effectChannel.Channel = fixture.Fixture.StartChannel + *channel.ChannelOffset
				}
				if fixture.PhaseOffset != nil {
					effectChannel.PhaseOffset = fixture.PhaseOffset
				}
				// Channel-level amplitude scale takes precedence over fixture-level
				if channel.AmplitudeScale != nil {
					effectChannel.AmplitudeScale = channel.AmplitudeScale
				} else if fixture.AmplitudeScale != nil {
					effectChannel.AmplitudeScale = fixture.AmplitudeScale
				}
				if channel.FrequencyScale != nil {
					effectChannel.FrequencyScale = channel.FrequencyScale
				}

				// If minValue and maxValue are set, calculate per-channel offset/amplitude
				// These override the base effect values and any amplitude scale
				if channel.MinValue != nil && channel.MaxValue != nil {
					// offset = center of oscillation = (min + max) / 2
					// amplitude = range from center to peak = (max - min) / 2
					offset := (*channel.MinValue + *channel.MaxValue) / 2
					amplitude := (*channel.MaxValue - *channel.MinValue) / 2
					effectChannel.Offset = &offset
					effectChannel.Amplitude = &amplitude
					// Clear amplitude scale since we're using absolute values
					effectChannel.AmplitudeScale = nil
				}

				effect.TargetChannels = append(effect.TargetChannels, effectChannel)
			}
		}
	}

	return effect
}

// parseEffectPriority converts priority band and sub priority to an EffectPriority.
func parseEffectPriority(band string, sub int) modulator.EffectPriority {
	var priorityBand modulator.PriorityBand
	switch band {
	case "BASE":
		priorityBand = modulator.PriorityBandBase
	case "USER":
		priorityBand = modulator.PriorityBandUser
	case "CUE":
		priorityBand = modulator.PriorityBandCue
	case "SYSTEM":
		priorityBand = modulator.PriorityBandSystem
	default:
		priorityBand = modulator.PriorityBandUser
	}
	return modulator.NewEffectPriority(priorityBand, sub)
}

// parseCompositionMode converts a string to CompositionMode.
func parseCompositionMode(mode string) modulator.CompositionMode {
	switch mode {
	case "OVERRIDE":
		return modulator.ComposeModeOverride
	case "ADDITIVE":
		return modulator.ComposeModeAdditive
	case "MULTIPLY":
		return modulator.ComposeModeMultiply
	default:
		return modulator.ComposeModeOverride
	}
}

// parseTransitionBehavior converts a string to TransitionBehavior.
func parseTransitionBehavior(behavior string) modulator.TransitionBehavior {
	switch behavior {
	case "FADE_OUT":
		return modulator.TransitionFadeOut
	case "PERSIST":
		return modulator.TransitionPersist
	case "SNAP_OFF":
		return modulator.TransitionSnapOff
	case "CROSSFADE_PARAMS":
		return modulator.TransitionCrossfadeParams
	default:
		return modulator.TransitionFadeOut
	}
}
