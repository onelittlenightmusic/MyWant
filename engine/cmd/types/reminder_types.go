package types

import (
	"fmt"
	. "mywant/engine/src"
	"strings"
	"time"
)

// ReminderPhase constants
const (
	ReminderPhaseWaiting   = "waiting"
	ReminderPhaseReaching  = "reaching"
	ReminderPhaseCompleted = "completed"
	ReminderPhaseFailed    = "failed"
)

// ReminderLocals holds type-specific local state for ReminderWant
type ReminderLocals struct {
	Message          string
	Ahead            string
	EventTime        time.Time
	DurationFromNow  string
	ReachingTime     time.Time
	RequireReaction  bool
	ReactionType     string
	Phase            string
	TimeoutSeconds   int
	LastCheckTime    time.Time
	monitor          *UserReactionMonitorAgent // Monitoring agent instance
}

// ReminderWant represents a want that sends reminders at scheduled times
type ReminderWant struct {
	Want
}

// NewReminderWant creates a new ReminderWant
func NewReminderWant(want *Want) *ReminderWant {
	return &ReminderWant{Want: *want}
}

// Initialize prepares the reminder want for execution
func (r *ReminderWant) Initialize() {
	r.StoreLog("[REMINDER] Initializing reminder: %s\n", r.Metadata.Name)

	// Initialize locals
	locals := &ReminderLocals{
		Phase:          ReminderPhaseWaiting,
		LastCheckTime:  time.Now(),
		TimeoutSeconds: 300, // 5 minutes default timeout
	}

	// Parse and validate parameters
	// Message (required)
	message, ok := r.Spec.Params["message"]
	if !ok || message == "" {
		r.StoreLog("ERROR: Missing required parameter 'message'")
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		r.StoreState("error_message", "Missing required parameter 'message'")
		r.Status = "failed"
		r.Locals = locals
		return
	}
	locals.Message = fmt.Sprintf("%v", message)

	// Ahead parameter (default: "5 minutes")
	ahead := "5 minutes"
	if aheadParam, ok := r.Spec.Params["ahead"]; ok {
		ahead = fmt.Sprintf("%v", aheadParam)
	}
	locals.Ahead = ahead

	// Parse ahead duration
	aheadDuration, err := parseDurationString(ahead)
	if err != nil {
		r.StoreLog(fmt.Sprintf("ERROR: Invalid ahead parameter '%s': %v", ahead, err))
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		r.StoreState("error_message", fmt.Sprintf("Invalid ahead parameter: %s", ahead))
		r.Status = "failed"
		r.Locals = locals
		return
	}

	// event_time parameter
	eventTimeStr, hasEventTime := r.Spec.Params["event_time"]

	// duration_from_now parameter
	durationFromNowStr, hasDurationFromNow := r.Spec.Params["duration_from_now"]

	// Check for mutually exclusive parameters
	if hasEventTime && eventTimeStr != "" && hasDurationFromNow && durationFromNowStr != "" {
		r.StoreLog("ERROR: Cannot provide both 'event_time' and 'duration_from_now'")
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		r.StoreState("error_message", "Cannot provide both 'event_time' and 'duration_from_now'")
		r.Status = "failed"
		r.Locals = locals
		return
	}

	var eventTime time.Time

	// Parse event_time if provided
	if hasEventTime && eventTimeStr != "" {
		var parseErr error
		eventTime, parseErr = time.Parse(time.RFC3339, fmt.Sprintf("%v", eventTimeStr))
		if parseErr != nil {
			r.StoreLog(fmt.Sprintf("ERROR: Invalid event_time format: %v", parseErr))
			r.StoreState("reminder_phase", ReminderPhaseFailed)
			r.StoreState("error_message", "Invalid event_time format (use RFC3339)")
			r.Status = "failed"
			r.Locals = locals
			return
		}
	} else if hasDurationFromNow && durationFromNowStr != "" {
		// Calculate event_time from duration_from_now
		durationStr := fmt.Sprintf("%v", durationFromNowStr)
		duration, parseErr := parseDurationString(durationStr)
		if parseErr != nil {
			r.StoreLog(fmt.Sprintf("ERROR: Invalid duration_from_now format: %v", parseErr))
			r.StoreState("reminder_phase", ReminderPhaseFailed)
			r.StoreState("error_message", fmt.Sprintf("Invalid duration_from_now format: %s", durationStr))
			r.Status = "failed"
			r.Locals = locals
			return
		}
		eventTime = time.Now().Add(duration)
	} else if !r.hasWhenSpec() {
		r.StoreLog("ERROR: Either 'event_time', 'duration_from_now', or 'when' spec must be provided")
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		r.StoreState("error_message", "Either 'event_time', 'duration_from_now', or 'when' spec must be provided")
		r.Status = "failed"
		r.Locals = locals
		return
	}

	// Store duration_from_now for reference
	if hasDurationFromNow && durationFromNowStr != "" {
		locals.DurationFromNow = fmt.Sprintf("%v", durationFromNowStr)
	}

	// Calculate reaching time if we have event_time
	if !eventTime.IsZero() {
		reachingTime := eventTime.Add(-aheadDuration)
		locals.ReachingTime = reachingTime
		locals.EventTime = eventTime

		// Check if event time is in the past
		if eventTime.Before(time.Now()) {
			r.StoreLog("[REMINDER] Event time is in the past, transitioning to reaching phase\n")
			locals.Phase = ReminderPhaseReaching
		}
	}

	// require_reaction parameter (default: false)
	requireReaction := false
	if reqReact, ok := r.Spec.Params["require_reaction"]; ok {
		if boolVal, ok := reqReact.(bool); ok {
			requireReaction = boolVal
		}
	}
	locals.RequireReaction = requireReaction

	// reaction_type parameter (default: "internal")
	reactionType := "internal"
	if rt, ok := r.Spec.Params["reaction_type"]; ok {
		reactionType = fmt.Sprintf("%v", rt)
	}
	locals.ReactionType = reactionType

	// Store initial state
	stateMap := map[string]any{
		"reminder_phase":   locals.Phase,
		"message":          locals.Message,
		"ahead":            locals.Ahead,
		"require_reaction": requireReaction,
		"reaction_type":    reactionType,
		"timeout":          300,
	}

	// Add duration_from_now if it was provided
	if locals.DurationFromNow != "" {
		stateMap["duration_from_now"] = locals.DurationFromNow
	}

	r.StoreStateMulti(stateMap)

	if !locals.ReachingTime.IsZero() {
		r.StoreStateMulti(map[string]any{
			"reaching_time": locals.ReachingTime.Format(time.RFC3339),
			"event_time":    locals.EventTime.Format(time.RFC3339),
		})
	}

	// Create monitoring agent during initialization (used if reaction is required)
	locals.monitor = NewUserReactionMonitorAgent()

	r.Locals = locals

	// Set up scheduler agent if we have when specs
	if r.hasWhenSpec() {
		r.StoreLog("Setting up scheduler for recurring reminder")
		// Scheduler agent will be set up by the caller
	}

	// If reaction is required, set Spec.Requires to trigger MonitorAgent and DoAgent
	if requireReaction {
		r.Spec.Requires = []string{
			"reminder_monitoring",        // MonitorAgent reads reactions via HTTP API
			"reminder_queue_management",  // DoAgent manages queue lifecycle (create/delete)
		}
	}

	r.StoreLog("[REMINDER] Initialized reminder '%s' with phase=%s, require_reaction=%v\n",
		r.Metadata.Name, locals.Phase, requireReaction)

	// Execute DoAgent to create reaction queue (synchronous)
	if len(r.Spec.Requires) > 0 {
		if err := r.ExecuteAgents(); err != nil {
			r.StoreLog(fmt.Sprintf("ERROR: Failed to execute agents: %v", err))
			r.StoreState("reminder_phase", ReminderPhaseFailed)
			r.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
			return
		}

		// Background monitoring agent will be started when transitioning to reaching phase
		// (see handlePhaseWaiting)
	}
}

// hasWhenSpec checks if this want has when specifications
func (r *ReminderWant) hasWhenSpec() bool {
	return r.Spec.When != nil && len(r.Spec.When) > 0
}

// IsAchieved checks if the reminder has been completed
func (r *ReminderWant) IsAchieved() bool {
	phase, _ := r.GetState("reminder_phase")
	return phase == ReminderPhaseCompleted
}

// Progress implements Progressable for ReminderWant
func (r *ReminderWant) Progress() {
	locals := r.getOrInitializeLocals()

	switch locals.Phase {
	case ReminderPhaseWaiting:
		r.handlePhaseWaiting(locals)

	case ReminderPhaseReaching:
		r.handlePhaseReaching(locals)

	case ReminderPhaseCompleted:
		// Already completed, nothing to do
		break

	case ReminderPhaseFailed:
		// Already failed, nothing to do
		break

	default:
		r.StoreLog(fmt.Sprintf("ERROR: Unknown phase: %s", locals.Phase))
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		locals.Phase = ReminderPhaseFailed
		r.Status = "failed"
		r.updateLocals(locals)
	}
}

// handlePhaseWaiting waits for the reaching time
func (r *ReminderWant) handlePhaseWaiting(locals *ReminderLocals) {
	if locals.ReachingTime.IsZero() {
		// No reaching time set, might be from when spec
		// Check if time has passed via scheduler
		return
	}

	// Check if reaching time has arrived
	now := time.Now()
	if now.After(locals.ReachingTime) {
		r.StoreLog(fmt.Sprintf("Reaching time arrived: %s", locals.ReachingTime.Format(time.RFC3339)))
		r.StoreState("reminder_phase", ReminderPhaseReaching)
		locals.Phase = ReminderPhaseReaching
		r.updateLocals(locals)

		// Start background monitoring agent when transitioning to reaching phase
		if locals.RequireReaction {
			queueIDValue, exists := r.GetState("reaction_queue_id")
			if exists && queueIDValue != nil && queueIDValue != "" {
				// Use pre-initialized monitor from locals
				agentName := "reaction-monitor-" + r.Metadata.ID
				monitor := locals.monitor

				if err := r.AddMonitoringAgent(agentName, 2*time.Second, monitor.Exec); err != nil {
					r.StoreLog(fmt.Sprintf("ERROR: Failed to start background monitoring: %v", err))
				} else {
					r.StoreLog("[REMINDER] Started background monitoring for %s\n", r.Metadata.Name)
				}
			}
		}
	}
}

// handlePhaseReaching handles the reaching phase
func (r *ReminderWant) handlePhaseReaching(locals *ReminderLocals) {
	// Check if user reaction is available
	if locals.RequireReaction {
		// Background monitoring agent continuously polls HTTP API for reactions
		// No need to call ExecuteAgents() here - monitoring runs in separate goroutine

		// Check state for user reaction (populated by MonitorAgent via HTTP API)
		if userReaction, exists := r.GetState("user_reaction"); exists {
			r.processReaction(locals, userReaction)
			return
		}

		// Check for timeout
		now := time.Now()
		if now.Unix()-locals.LastCheckTime.Unix() > int64(locals.TimeoutSeconds) {
			r.StoreLog(fmt.Sprintf("Reminder reaction timeout after %d seconds", locals.TimeoutSeconds))
			r.handleTimeout(locals)
			return
		}
	} else {
		// No reaction required, check if event time has passed
		if !locals.EventTime.IsZero() && time.Now().After(locals.EventTime) {
			r.StoreLog(fmt.Sprintf("Event time passed, completing reminder"))
			r.StoreState("reminder_phase", ReminderPhaseCompleted)
			r.StoreState("auto_completed", true)
			locals.Phase = ReminderPhaseCompleted
			r.Status = "achieved"
			r.updateLocals(locals)
			return
		}

		// For reminders without reaction required and without explicit event_time,
		// complete after 10 seconds of reaching
		if now := time.Now(); now.After(locals.LastCheckTime.Add(10 * time.Second)) {
			r.StoreLog("Completing reminder (no reaction required)")
			r.StoreState("reminder_phase", ReminderPhaseCompleted)
			r.StoreState("auto_completed", true)
			locals.Phase = ReminderPhaseCompleted
			r.Status = "achieved"
			r.updateLocals(locals)
		}
	}
}

// processReaction processes user reaction
func (r *ReminderWant) processReaction(locals *ReminderLocals, reactionData any) {
	// Handle reaction data
	if reactionMap, ok := reactionData.(map[string]any); ok {
		if approved, ok := reactionMap["approved"].(bool); ok {
			if approved {
				r.StoreLog("Reminder approved by user")
				r.StoreState("reminder_phase", ReminderPhaseCompleted)
				r.StoreState("reaction_result", "approved")
				locals.Phase = ReminderPhaseCompleted
				r.Status = "achieved"
			} else {
				r.StoreLog("Reminder rejected by user")
				r.StoreState("reminder_phase", ReminderPhaseFailed)
				r.StoreState("reaction_result", "rejected")
				locals.Phase = ReminderPhaseFailed
				r.Status = "failed"
			}
			r.updateLocals(locals)
		}
	}
}

// handleTimeout handles reaction timeout
func (r *ReminderWant) handleTimeout(locals *ReminderLocals) {
	if locals.RequireReaction {
		r.StoreLog("Reaction timeout - marking as failed (require_reaction=true)")
		r.StoreState("reminder_phase", ReminderPhaseFailed)
		r.StoreState("timeout", true)
		locals.Phase = ReminderPhaseFailed
		r.Status = "failed"
	} else {
		r.StoreLog("Reaction timeout - auto-completing (require_reaction=false)")
		r.StoreState("reminder_phase", ReminderPhaseCompleted)
		r.StoreState("auto_completed", true)
		locals.Phase = ReminderPhaseCompleted
		r.Status = "achieved"
	}
	r.updateLocals(locals)
}

// getOrInitializeLocals retrieves or initializes the locals
func (r *ReminderWant) getOrInitializeLocals() *ReminderLocals {
	if r.Locals != nil {
		if locals, ok := r.Locals.(*ReminderLocals); ok {
			return locals
		}
	}

	// Initialize from state
	locals := &ReminderLocals{
		Phase:         ReminderPhaseWaiting,
		LastCheckTime: time.Now(),
	}

	if phase, exists := r.GetState("reminder_phase"); exists {
		if phaseStr, ok := phase.(string); ok {
			locals.Phase = phaseStr
		}
	}

	if message, exists := r.GetState("message"); exists {
		locals.Message = fmt.Sprintf("%v", message)
	}

	if ahead, exists := r.GetState("ahead"); exists {
		locals.Ahead = fmt.Sprintf("%v", ahead)
	}

	if reqReact, exists := r.GetState("require_reaction"); exists {
		if boolVal, ok := reqReact.(bool); ok {
			locals.RequireReaction = boolVal
		}
	}

	if rtStr, exists := r.GetState("reaching_time"); exists {
		if rtTime, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", rtStr)); err == nil {
			locals.ReachingTime = rtTime
		}
	}

	if etStr, exists := r.GetState("event_time"); exists {
		if etTime, err := time.Parse(time.RFC3339, fmt.Sprintf("%v", etStr)); err == nil {
			locals.EventTime = etTime
		}
	}

	return locals
}

// updateLocals updates the in-memory locals
func (r *ReminderWant) updateLocals(locals *ReminderLocals) {
	r.Locals = locals
}

// parseDurationString parses duration strings like "5 minutes", "10 seconds", etc.
func parseDurationString(s string) (time.Duration, error) {
	// Simple parser for common formats
	var unit time.Duration

	// Parse from the end to find the unit
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	// Extract number and unit
	var numStr string
	for i, c := range s {
		if c >= '0' && c <= '9' {
			numStr += string(c)
		} else {
			s = s[i:]
			break
		}
	}

	if numStr == "" {
		return 0, fmt.Errorf("no number found in duration: %s", s)
	}

	// Parse the unit
	s = strings.TrimSpace(s)
	switch s {
	case "second", "seconds":
		unit = time.Second
	case "minute", "minutes":
		unit = time.Minute
	case "hour", "hours":
		unit = time.Hour
	case "day", "days":
		unit = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unknown duration unit: %s", s)
	}

	// Parse the number
	var num int
	_, err := fmt.Sscanf(numStr, "%d", &num)
	if err != nil {
		return 0, fmt.Errorf("invalid number in duration: %s", numStr)
	}

	return time.Duration(num) * unit, nil
}

// RegisterReminderWantType registers the ReminderWant type with the ChainBuilder
func RegisterReminderWantType(builder *ChainBuilder) {
	builder.RegisterWantType("reminder", func(metadata Metadata, spec WantSpec) Progressable {
		want := &Want{
			Metadata: metadata,
			Spec:     spec,
		}
		return NewReminderWant(want)
	})
}
