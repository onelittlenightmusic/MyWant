package types

import (
	"fmt"
	mywant "mywant/engine/core"
)

func init() {
	mywant.RegisterWantImplementation[GmailDynamicWant, GmailDynamicLocals]("email_dynamic")
}

// GmailDynamicPhase represents the current phase of the dynamic want lifecycle
type GmailDynamicPhase string

const (
	PhaseDiscovery  GmailDynamicPhase = "discovery"  // Goose/Gemini discovers MCP tools
	PhaseCoding     GmailDynamicPhase = "coding"     // Gemini generates Go code
	PhaseCompiling  GmailDynamicPhase = "compiling"  // Compiler compiles Go to WASM
	PhaseValidation GmailDynamicPhase = "validation" // Test execution with direct MCP call
	PhaseStable     GmailDynamicPhase = "stable"     // Fixed WASM used for execution

	MaxRetriesPerPhase = 3 // Maximum retries for an agent in a given phase
)

// GmailDynamicLocals holds type-specific local state for GmailDynamicWant
type GmailDynamicLocals struct {
	// State fields (auto-synced)
	RawSamples any `mywant:"internal,raw_samples"`
}

// GmailDynamicWant represents a self-evolving Gmail want
type GmailDynamicWant struct {
	mywant.Want
}

func (g *GmailDynamicWant) GetLocals() *GmailDynamicLocals {
	return mywant.CheckLocalsInitialized[GmailDynamicLocals](&g.Want)
}

func (g *GmailDynamicWant) GetPhase() string {
	return mywant.GetCurrent(g, "phase", string(PhaseDiscovery))
}

func (g *GmailDynamicWant) SetPhase(phase string) {
	g.SetCurrent("phase", phase)
}

func (g *GmailDynamicWant) GetErrorFeedback() string {
	return mywant.GetCurrent(g, "error_feedback", "")
}

func (g *GmailDynamicWant) Initialize() {
	g.StoreLog("[GMAIL-DYNAMIC] Initializing dynamic want: %s", g.Metadata.Name)

	// Initialize PhaseRetryCount map
	if g.PhaseRetryCount == nil {
		g.PhaseRetryCount = make(map[string]int)
	}

	if g.GetPhase() == "" {
		g.SetPhase(string(PhaseDiscovery))
	}
}

func (g *GmailDynamicWant) IsAchieved() bool {
	return g.GetPhase() == string(PhaseStable)
}

func (g *GmailDynamicWant) Progress() {
	if g.GetStatus() == mywant.WantStatusFailed || g.GetStatus() == mywant.WantStatusTerminated {
		return
	}

	locals := g.GetLocals()
	phaseStr := g.GetPhase()
	phase := GmailDynamicPhase(phaseStr)

	// Only log phase transition or significant events to avoid spam
	lastLoggedPhase := mywant.GetCurrent(g, "last_logged_phase", "")
	if lastLoggedPhase != phaseStr {
		g.StoreLog("[GMAIL-DYNAMIC] Transitioned to phase: %s", phase)
		g.SetCurrent("last_logged_phase", phaseStr)
	}

	// Get current retry count for this phase
	currentRetries := g.PhaseRetryCount[string(phase)]

	// Before executing any agent, check if we've exceeded max retries
	if currentRetries >= MaxRetriesPerPhase {
		g.SetStatus(mywant.WantStatusFailed)
		g.StoreLog("[GMAIL-DYNAMIC][CRITICAL] Terminating: Failed in phase %s after %d retries. Detail: %s",
			phase, currentRetries, g.GetErrorFeedback())
		return
	}

	var err error
	switch phase {
	case PhaseDiscovery:
		err = g.handleDiscovery(locals)
	case PhaseCoding:
		err = g.handleCoding(locals)
	case PhaseCompiling:
		err = g.handleCompiling(locals)
	case PhaseValidation:
		err = g.handleValidation(locals)
	case PhaseStable:
		return
	}

	if err != nil {
		// Increment retry count
		g.PhaseRetryCount[string(phase)] = currentRetries + 1
		g.LastPhaseError = err.Error()

		errorMsg := fmt.Sprintf("Agent failed: %v", err)
		errorFeedback := g.GetErrorFeedback()
		if errorFeedback != "" {
			errorMsg = fmt.Sprintf("%s | Compiler Output: %s", errorMsg, errorFeedback)
		}

		g.StoreLog("[GMAIL-DYNAMIC][RETRY %d/%d] Phase %s: %s",
			g.PhaseRetryCount[string(phase)], MaxRetriesPerPhase, phase, errorMsg)

		g.SetStatus(mywant.WantStatusReaching)
		return
	}

	// Success: Reset retries for the phase
	if currentRetries > 0 {
		g.PhaseRetryCount[string(phase)] = 0
		g.LastPhaseError = ""
	}
	g.SetStatus(mywant.WantStatusReaching)
}

func (g *GmailDynamicWant) handleDiscovery(locals *GmailDynamicLocals) error {
	g.StoreLog("[PHASE:DISCOVERY] Requesting tool discovery via Goose/Gemini")

	if err := g.ExecuteAgents(); err != nil {
		return fmt.Errorf("Discovery Agent failed: %w", err)
	}

	// Check if samples were collected
	if locals.RawSamples != nil {
		g.SetPhase(string(PhaseCoding))
		g.StoreLog("[PHASE:DISCOVERY] Samples collected. Moving to PhaseCoding.")
	} else {
		return fmt.Errorf("Discovery Agent did not return raw_samples")
	}
	return nil
}

func (g *GmailDynamicWant) handleCoding(locals *GmailDynamicLocals) error {
	errorFeedback := g.GetErrorFeedback()
	if errorFeedback != "" {
		g.StoreLog("[PHASE:CODING] Re-generating Go code with error feedback")
	} else {
		g.StoreLog("[PHASE:CODING] Generating Go code for WASM plugin")
	}

	if err := g.ExecuteAgents(); err != nil {
		return fmt.Errorf("Developer Agent failed: %w", err)
	}

	sourceCode := mywant.GetCurrent(g, "source_code", "")
	if sourceCode != "" {
		g.SetPhase(string(PhaseCompiling))
		g.StoreLog("[PHASE:CODING] Code generated. Moving to PhaseCompiling.")
	} else {
		return fmt.Errorf("Developer Agent did not return source_code")
	}
	return nil
}

func (g *GmailDynamicWant) handleCompiling(locals *GmailDynamicLocals) error {
	g.StoreLog("[PHASE:COMPILING] Compiling Go code to WASM")

	if err := g.ExecuteAgents(); err != nil {
		errorFeedback := g.GetErrorFeedback()
		g.StoreLog("[PHASE:COMPILING][ERROR] Compilation failed: %s", errorFeedback)

		// Check current retry count for compiling phase
		currentRetries := g.PhaseRetryCount[string(PhaseCompiling)]

		// If this will be the 3rd failure, don't go back to coding - just fail
		if currentRetries+1 >= MaxRetriesPerPhase {
			g.StoreLog("[PHASE:COMPILING][CRITICAL] Compilation failed %d times. Want will be marked as Failed.", currentRetries+1)
			g.SetStatus(mywant.WantStatusFailed)
			return nil // Don't return error - State needs to be committed
		}

		// Increment retry count and go back to coding phase with feedback for regeneration
		g.PhaseRetryCount[string(PhaseCompiling)] = currentRetries + 1
		g.SetPhase(string(PhaseCoding))
		g.StoreLog("[PHASE:COMPILING] Moving back to PhaseCoding with error feedback for code regeneration (attempt %d/%d)", currentRetries+1, MaxRetriesPerPhase)
		return nil // Don't return error - let State be committed and retry in next loop
	}

	wasmPath := mywant.GetCurrent(g, "wasm_path", "")
	if wasmPath != "" {
		g.SetPhase(string(PhaseValidation))
		g.StoreLog("[PHASE:COMPILING] WASM compiled at %s. Moving to PhaseValidation.", wasmPath)
		// Clear error_feedback on success
		g.SetCurrent("error_feedback", "")
	} else {
		return fmt.Errorf("Compiler Agent did not return wasm_path")
	}
	return nil
}

func (g *GmailDynamicWant) handleValidation(locals *GmailDynamicLocals) error {
	g.StoreLog("[PHASE:VALIDATION] Validating WASM logic with direct MCP call")

	if err := g.ExecuteAgents(); err != nil {
		errorFeedback := g.GetErrorFeedback()
		g.StoreLog("[PHASE:VALIDATION][ERROR] Validation failed: %s", errorFeedback)

		// Check current retry count for validation phase
		currentRetries := g.PhaseRetryCount[string(PhaseValidation)]

		// If this will be the 3rd failure, don't go back to coding - just fail
		if currentRetries+1 >= MaxRetriesPerPhase {
			g.StoreLog("[PHASE:VALIDATION][CRITICAL] Validation failed %d times. Want will be marked as Failed.", currentRetries+1)
			return fmt.Errorf("Validator Agent failed after %d attempts: %w. Feedback: %s", currentRetries+1, err, errorFeedback)
		}

		// Go back to coding phase for regeneration
		g.SetPhase(string(PhaseCoding))
		g.StoreLog("[PHASE:VALIDATION] Moving back to PhaseCoding with error feedback for code regeneration (attempt %d/%d)", currentRetries+1, MaxRetriesPerPhase)
		return fmt.Errorf("Validator Agent failed: %w. Feedback: %s", err, errorFeedback)
	}

	validationSuccess := mywant.GetCurrent(g, "validation_success", false)
	if validationSuccess {
		g.SetPhase(string(PhaseStable))
		g.StoreLog("[PHASE:VALIDATION] Success! Moving to PhaseStable.")
		// Clear error_feedback on success
		g.SetCurrent("error_feedback", "")
	} else {
		return fmt.Errorf("Validator Agent did not report validation_success")
	}
	return nil
}

func (g *GmailDynamicWant) handleStable() {
	// Re-use the existing WASM for any incoming prompts
	g.StoreLog("[PHASE:STABLE] Using optimized WASM logic for execution")
}
