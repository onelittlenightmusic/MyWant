package types

import (
	"fmt"
	mywant "mywant/engine/src"
)

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
type GmailDynamicLocals struct{}

// GmailDynamicWant represents a self-evolving Gmail want
type GmailDynamicWant struct {
	mywant.Want
}

func (g *GmailDynamicWant) GetLocals() *GmailDynamicLocals {
	return mywant.GetLocals[GmailDynamicLocals](&g.Want)
}

func init() {
	mywant.RegisterWantImplementation[GmailDynamicWant, GmailDynamicLocals]("gmail_dynamic")
}

func (g *GmailDynamicWant) Initialize() {
	fmt.Printf("[GMAIL-DYNAMIC] Initialize() called for: %s\n", g.Metadata.Name)
	g.StoreLog("[GMAIL-DYNAMIC] Initializing dynamic want: %s", g.Metadata.Name)

	// Initialize PhaseRetryCount map
	if g.PhaseRetryCount == nil {
		g.PhaseRetryCount = make(map[string]int)
	}

	phase, _ := g.GetStateString("phase", "")
	if phase == "" {
		fmt.Printf("[GMAIL-DYNAMIC] Setting initial phase to discovery for: %s\n", g.Metadata.Name)
		g.StoreState("phase", string(PhaseDiscovery))
	} else {
		fmt.Printf("[GMAIL-DYNAMIC] Existing phase: %s for: %s\n", phase, g.Metadata.Name)
	}
}

func (g *GmailDynamicWant) IsAchieved() bool {
	phase, _ := g.GetStateString("phase", "")
	return phase == string(PhaseStable)
}

func (g *GmailDynamicWant) Progress() {
	fmt.Printf("[GMAIL-DYNAMIC] Progress() called for: %s\n", g.Metadata.Name)
	if g.GetStatus() == mywant.WantStatusFailed || g.GetStatus() == mywant.WantStatusTerminated {
		fmt.Printf("[GMAIL-DYNAMIC] Skipping - status is %s\n", g.GetStatus())
		return
	}

	phaseStr, _ := g.GetStateString("phase", string(PhaseDiscovery))
	phase := GmailDynamicPhase(phaseStr)
	fmt.Printf("[GMAIL-DYNAMIC] Current phase: %s for: %s\n", phase, g.Metadata.Name)

	// Only log phase transition or significant events to avoid spam
	lastLoggedPhase, _ := g.GetStateString("last_logged_phase", "")
	if lastLoggedPhase != phaseStr {
		g.StoreLog("[GMAIL-DYNAMIC] Transitioned to phase: %s", phase)
		g.StoreState("last_logged_phase", phaseStr)
	}

	// Get current retry count for this phase
	currentRetries := g.PhaseRetryCount[string(phase)]

	// Before executing any agent, check if we've exceeded max retries
	if currentRetries >= MaxRetriesPerPhase {
		g.SetStatus(mywant.WantStatusFailed)
		feedback, _ := g.GetStateString("error_feedback", "No detailed feedback")
		g.StoreLog("[GMAIL-DYNAMIC][CRITICAL] Terminating: Failed in phase %s after %d retries. Detail: %s",
			phase, currentRetries, feedback)
		// Explicitly print to server log for visibility
		fmt.Printf("[GMAIL-DYNAMIC-FAILURE] %s failed in phase %s. Error: %s\n", g.Metadata.Name, phase, feedback)
		return
	}

	var err error
	switch phase {
	case PhaseDiscovery:
		err = g.handleDiscovery()
	case PhaseCoding:
		err = g.handleCoding()
	case PhaseCompiling:
		err = g.handleCompiling()
	case PhaseValidation:
		err = g.handleValidation()
	case PhaseStable:
		return
	}

	if err != nil {
		// Increment retry count
		g.PhaseRetryCount[string(phase)] = currentRetries + 1
		g.LastPhaseError = err.Error()
		
		feedback, _ := g.GetStateString("error_feedback", "")
		errorMsg := fmt.Sprintf("Agent failed: %v", err)
		if feedback != "" {
			errorMsg = fmt.Sprintf("%s | Compiler Output: %s", errorMsg, feedback)
		}

		g.StoreLog("[GMAIL-DYNAMIC][RETRY %d/%d] Phase %s: %s", 
			g.PhaseRetryCount[string(phase)], MaxRetriesPerPhase, phase, errorMsg)
		
		// Print detailed error to server console immediately
		fmt.Printf("[GMAIL-DYNAMIC-RETRY] %s (%s): %s\n", g.Metadata.Name, phase, errorMsg)
		
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

func (g *GmailDynamicWant) handleDiscovery() error {
	g.StoreLog("[PHASE:DISCOVERY] Requesting tool discovery via Goose/Gemini")
	
	// Requirement for the Discovery Agent
	g.Spec.Requires = []string{"mcp_dynamic_discovery"}
	
	if err := g.ExecuteAgents(); err != nil {
		return fmt.Errorf("Discovery Agent failed: %w", err)
	}

	// Check if samples were collected
	samples, exists := g.GetState("raw_samples")
	if exists && samples != nil {
		g.StoreState("phase", string(PhaseCoding))
		g.StoreLog("[PHASE:DISCOVERY] Samples collected. Moving to PhaseCoding.")
	} else {
		return fmt.Errorf("Discovery Agent did not return raw_samples")
	}
	return nil
}

func (g *GmailDynamicWant) handleCoding() error {
	feedback, _ := g.GetStateString("error_feedback", "")
	if feedback != "" {
		g.StoreLog("[PHASE:CODING] Re-generating Go code with error feedback: %s", feedback)
		fmt.Printf("[GMAIL-DYNAMIC] Coding with feedback: %s\n", feedback)
	} else {
		g.StoreLog("[PHASE:CODING] Generating Go code for WASM plugin")
		fmt.Printf("[GMAIL-DYNAMIC] Starting code generation\n")
	}

	g.Spec.Requires = []string{"mcp_dynamic_developer"}

	fmt.Printf("[GMAIL-DYNAMIC] Executing Developer Agent\n")
	if err := g.ExecuteAgents(); err != nil {
		fmt.Printf("[GMAIL-DYNAMIC] Developer Agent failed: %v\n", err)
		return fmt.Errorf("Developer Agent failed: %w", err)
	}

	source, exists := g.GetState("source_code")
	fmt.Printf("[GMAIL-DYNAMIC] Checking source_code: exists=%v, empty=%v\n", exists, source == "" || source == nil)
	if exists && source != "" {
		g.StoreState("phase", string(PhaseCompiling))
		g.StoreLog("[PHASE:CODING] Code generated. Moving to PhaseCompiling.")
		fmt.Printf("[GMAIL-DYNAMIC] Transitioning to PhaseCompiling\n")
	} else {
		fmt.Printf("[GMAIL-DYNAMIC] ERROR: source_code not found or empty\n")
		return fmt.Errorf("Developer Agent did not return source_code")
	}
	return nil
}

func (g *GmailDynamicWant) handleCompiling() error {
	g.StoreLog("[PHASE:COMPILING] Compiling Go code to WASM")

	g.Spec.Requires = []string{"mcp_dynamic_compiler"}

	if err := g.ExecuteAgents(); err != nil {
		feedback, _ := g.GetStateString("error_feedback", "")
		g.StoreLog("[PHASE:COMPILING][ERROR] Compilation failed: %s", feedback)

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
		g.StoreState("phase", string(PhaseCoding))
		g.StoreLog("[PHASE:COMPILING] Moving back to PhaseCoding with error feedback for code regeneration (attempt %d/%d)", currentRetries+1, MaxRetriesPerPhase)
		return nil // Don't return error - let State be committed and retry in next loop
	}

	wasmPath, exists := g.GetStateString("wasm_path", "")
	if exists && wasmPath != "" {
		g.StoreState("phase", string(PhaseValidation))
		g.StoreLog("[PHASE:COMPILING] WASM compiled at %s. Moving to PhaseValidation.", wasmPath)
		// Clear error_feedback on success
		g.StoreState("error_feedback", "")
	} else {
		return fmt.Errorf("Compiler Agent did not return wasm_path")
	}
	return nil
}

func (g *GmailDynamicWant) handleValidation() error {
	g.StoreLog("[PHASE:VALIDATION] Validating WASM logic with direct MCP call")

	// Here we use the WasmManager to run the code
	// and check if it successfully communicates with the Gmail MCP server

	g.Spec.Requires = []string{"mcp_dynamic_validator"}

	if err := g.ExecuteAgents(); err != nil {
		feedback, _ := g.GetStateString("error_feedback", "")
		g.StoreLog("[PHASE:VALIDATION][ERROR] Validation failed: %s", feedback)

		// Check current retry count for validation phase
		currentRetries := g.PhaseRetryCount[string(PhaseValidation)]

		// If this will be the 3rd failure, don't go back to coding - just fail
		if currentRetries+1 >= MaxRetriesPerPhase {
			g.StoreLog("[PHASE:VALIDATION][CRITICAL] Validation failed %d times. Want will be marked as Failed.", currentRetries+1)
			// Don't change phase - let Progress() handle the failure
			return fmt.Errorf("Validator Agent failed after %d attempts: %w. Feedback: %s", currentRetries+1, err, feedback)
		}

		// Go back to coding phase for regeneration
		g.StoreState("phase", string(PhaseCoding))
		g.StoreLog("[PHASE:VALIDATION] Moving back to PhaseCoding with error feedback for code regeneration (attempt %d/%d)", currentRetries+1, MaxRetriesPerPhase)
		return fmt.Errorf("Validator Agent failed: %w. Feedback: %s", err, feedback)
	}

	success, _ := g.GetStateBool("validation_success", false)
	if success {
		g.StoreState("phase", string(PhaseStable))
		g.StoreLog("[PHASE:VALIDATION] Success! Moving to PhaseStable.")
		// Clear error_feedback on success
		g.StoreState("error_feedback", "")
	} else {
		return fmt.Errorf("Validator Agent did not report validation_success")
	}
	return nil
}

func (g *GmailDynamicWant) handleStable() {
	// Re-use the existing WASM for any incoming prompts
	g.StoreLog("[PHASE:STABLE] Using optimized WASM logic for execution")
	
	// Final result handling...
}
