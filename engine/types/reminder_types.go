package types

import (
	"fmt"
	. "mywant/engine/core"
	"strings"
	"time"
)

func init() {
	RegisterWantImplementation[ReminderWant, ReminderLocals]("reminder")
}

// ReminderPhase constants
const (
	ReminderPhaseWaiting   = "waiting"
	ReminderPhaseReaching  = "reaching"
	ReminderPhaseCompleted = "completed"
	ReminderPhaseFailed    = "failed"
)

// ReminderLocals holds type-specific local state for ReminderWant
type ReminderLocals struct {
	Message         string
	Ahead           string
	EventTime       time.Time
	DurationFromNow string
	ReachingTime    time.Time
	RequireReaction bool
	ReactionType    string
	Phase           string
	TimeoutSeconds  int
	LastCheckTime   time.Time
}

// ReminderWant represents a want that sends reminders at scheduled times
type ReminderWant struct {
	Want
}

func (r *ReminderWant) GetLocals() *ReminderLocals {
	return CheckLocalsInitialized[ReminderLocals](&r.Want)
}

// Initialize prepares the reminder want for execution
func (r *ReminderWant) Initialize() {
	r.StoreLog("[REMINDER] Initializing reminder: %s", r.Metadata.Name)

	// CRITICAL: Stop any existing background agents (like monitor) before fresh start
	// This ensures we don't have multiple goroutines monitoring different queue IDs
	if err := r.StopAllBackgroundAgents(); err != nil {
		r.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	// Get locals (guaranteed to be initialized by framework)
	locals := r.GetLocals()
	locals.Phase = ReminderPhaseWaiting
	locals.LastCheckTime = time.Now()
	locals.TimeoutSeconds = 300 // 5 minutes default timeout

	// Parse and validate parameters using ConfigError pattern
	// Message (required)
	locals.Message = r.GetStringParam("message", "")
	if locals.Message == "" {
		r.SetConfigError("message", "Missing required parameter 'message'")
		return
	}

	// Ahead parameter (default: "5 minutes")
	locals.Ahead = r.GetStringParam("ahead", "5 minutes")

	// Parse ahead duration
	aheadDuration, err := parseDurationString(locals.Ahead)
	if err != nil {
		r.SetConfigError("ahead", fmt.Sprintf("Invalid ahead parameter '%s': %v", locals.Ahead, err))
		return
	}

	// event_time parameter
	eventTimeStr := r.GetStringParam("event_time", "")

	// duration_from_now parameter
	durationFromNowStr := r.GetStringParam("duration_from_now", "")

	// Check for mutually exclusive parameters
	if eventTimeStr != "" && durationFromNowStr != "" {
		r.SetConfigError("event_time/duration_from_now", "Cannot provide both 'event_time' and 'duration_from_now'")
		return
	}

	var eventTime time.Time

	// Parse event_time if provided
	if eventTimeStr != "" {
		var parseErr error
		eventTime, parseErr = time.Parse(time.RFC3339, eventTimeStr)
		if parseErr != nil {
			r.SetConfigError("event_time", fmt.Sprintf("Invalid event_time format (use RFC3339): %v", parseErr))
			return
		}
	} else if durationFromNowStr != "" {
		// Calculate event_time from duration_from_now
		duration, parseErr := parseDurationString(durationFromNowStr)
		if parseErr != nil {
			r.SetConfigError("duration_from_now", fmt.Sprintf("Invalid duration_from_now format: %v", parseErr))
			return
		}
		eventTime = time.Now().Add(duration)
		locals.DurationFromNow = durationFromNowStr
	} else if !r.hasWhenSpec() {
		r.SetConfigError("timing", "Either 'event_time', 'duration_from_now', or 'when' spec must be provided")
		return
	}

	// Calculate reaching time if we have event_time
	if !eventTime.IsZero() {
		reachingTime := eventTime.Add(-aheadDuration)
		locals.ReachingTime = reachingTime
		locals.EventTime = eventTime

		// Check if event time is in the past
		if eventTime.Before(time.Now()) {
			locals.Phase = ReminderPhaseReaching
		}
	}

	// require_reaction parameter (default: false)
	locals.RequireReaction = r.GetBoolParam("require_reaction", false)

	// reaction_type parameter (default: "internal")
	locals.ReactionType = r.GetStringParam("reaction_type", "internal")

	// Store initial state
	stateMap := map[string]any{
		"reminder_phase":           locals.Phase,
		"message":                  locals.Message,
		"ahead":                    locals.Ahead,
		"require_reaction":         locals.RequireReaction,
		"reaction_type":            locals.ReactionType,
		"timeout":                  300,
		"reaction_queue_id":        "",    // ALWAYS clear on fresh initialization to ensure new queue
		"_reaction_packet_emitted": false, // Reset emission flag for new cycle
		"user_reaction":            nil,   // Clear previous user reaction
		"reaction_result":          "",    // Clear previous reaction result
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

	r.Locals = locals

	// Set up scheduler agent if we have when specs
	if r.hasWhenSpec() {
		// Scheduler agent will be set up by the caller
	}

	// Execute DoAgent to create reaction queue (synchronous)
	if locals.RequireReaction {
		if err := r.ExecuteAgents(); err != nil {
			r.StoreLog("ERROR: Failed to execute agents: %v", err)
			r.StoreState("reminder_phase", ReminderPhaseFailed)
			r.StoreState("error_message", fmt.Sprintf("Agent execution failed: %v", err))
			return
		}

		// Start background monitoring agent as soon as queue is created
		r.startMonitoringIfNeeded()
	}
}

// startMonitoringIfNeeded starts the reaction monitor agent if not already running
func (r *ReminderWant) startMonitoringIfNeeded() {
	locals := r.GetLocals()
	if locals == nil || !locals.RequireReaction {
		return
	}

	if queueID, ok := r.GetStateString("reaction_queue_id", ""); ok && queueID != "" {
		agentName := "reaction-monitor-" + r.Metadata.ID
		if _, exists := r.GetBackgroundAgent(agentName); !exists {
			registry := r.GetAgentRegistry()
			typeDef := r.WantTypeDefinition
			if registry != nil && typeDef != nil {
				for _, monCap := range typeDef.MonitorCapabilities {
					agents := registry.FindMonitorAgentsByCapabilityName(monCap.Capability)
					if len(agents) == 0 {
						continue
					}
					if err := r.AddMonitoringAgent(agentName, 2*time.Second, agents[0].Exec); err != nil {
						r.StoreLog("ERROR: Failed to start background monitoring: %v", err)
					}
					break
				}
			}
		}
	}
}

// hasWhenSpec checks if this want has when specifications
func (r *ReminderWant) hasWhenSpec() bool {
	return r.Spec.When != nil && len(r.Spec.When) > 0
}

// IsAchieved checks if the reminder has been completed
func (r *ReminderWant) IsAchieved() bool {
	phase, _ := r.GetStateString("reminder_phase", "")
	return phase == ReminderPhaseCompleted
}

// CalculateAchievingPercentage returns the progress percentage
func (r *ReminderWant) CalculateAchievingPercentage() int {
	if r.IsAchieved() || r.Status == WantStatusAchieved || r.Status == WantStatusFailed {
		return 100
	}
	phase, _ := r.GetState("reminder_phase")
	switch phase {
	case ReminderPhaseWaiting:
		return 10
	case ReminderPhaseReaching:
		return 50
	case ReminderPhaseCompleted, ReminderPhaseFailed:
		return 100
	default:
		return 0
	}
}

// Progress implements Progressable for ReminderWant
func (r *ReminderWant) Progress() {
	locals := r.GetLocals()

	// Update achieving percentage based on current phase
	r.StoreState("achieving_percentage", r.CalculateAchievingPercentage())

	// Calculate and store time remaining
	timeRemaining := r.calculateTimeRemaining(locals)
	r.StoreState("time_remaining", timeRemaining)

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
		r.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", locals.Phase))
	}
}

// calculateTimeRemaining calculates the time remaining until the next event
func (r *ReminderWant) calculateTimeRemaining(locals *ReminderLocals) string {
	now := time.Now()

	switch locals.Phase {
	case ReminderPhaseWaiting:
		// Time until reaching_time
		if !locals.ReachingTime.IsZero() {
			duration := locals.ReachingTime.Sub(now)
			if duration < 0 {
				return "0s"
			}
			return formatDuration(duration)
		}
		return ""

	case ReminderPhaseReaching:
		// Time until event_time
		if !locals.EventTime.IsZero() {
			duration := locals.EventTime.Sub(now)
			if duration < 0 {
				return "0s"
			}
			return formatDuration(duration)
		}
		return ""

	case ReminderPhaseCompleted, ReminderPhaseFailed:
		// No time remaining for completed or failed reminders
		return "0s"

	default:
		return ""
	}
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}

	// Round to seconds
	d = d.Round(time.Second)

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}

	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if minutes == 0 && seconds == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	if seconds == 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
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
		// Clear existing queue ID to force creation of a new one for this new cycle
		r.StoreStateMulti(map[string]any{
			"reminder_phase":           ReminderPhaseReaching,
			"reaction_queue_id":        "",
			"_reaction_packet_emitted": false,
			"user_reaction":            nil, // Clear previous reaction
			"reaction_result":          "",  // Clear previous result
		})

		locals.Phase = ReminderPhaseReaching

		// Trigger agent to create new queue immediately
		r.ExecuteAgents()
	}
}

// emitReactionPacketIfNeeded sends the reaction request packet to connected users
func (r *ReminderWant) emitReactionPacketIfNeeded(locals *ReminderLocals) {
	if !locals.RequireReaction {
		return
	}

	// Check if already emitted in this state
	emitted, _ := r.GetStateBool("_reaction_packet_emitted", false)
	if emitted {
		return
	}

	if queueID, ok := r.GetStateString("reaction_queue_id", ""); ok && queueID != "" {
		// Emit reaction request packet to connected users (silencers)
		packet := map[string]any{
			"reaction_id":   queueID,
			"reaction_type": locals.ReactionType,
			"source_want":   r.Metadata.Name,
		}
		r.Provide(packet)
		r.StoreState("_reaction_packet_emitted", true)
	}
}

// handlePhaseReaching handles the reaching phase
func (r *ReminderWant) handlePhaseReaching(locals *ReminderLocals) {
	// If this is a recurring reminder and we are in reaching phase again,
	// we might need to reset the queue ID if the previous one was already processed.
	// However, for recurring reminders triggered by scheduler, the want status
	// is reset to Idle then Reaching, which calls Initialize().
	// But our test showed it might stay in reaching status.

	// Ensure packet is emitted (handles both transition and restart-while-reaching)
	r.emitReactionPacketIfNeeded(locals)

	// Set WaitingUserAction status when waiting for user reaction
	if locals.RequireReaction && r.GetStatus() != WantStatusWaitingUserAction {
		r.SetStatus(WantStatusWaitingUserAction)
	}

	// Check if user reaction is available
	if locals.RequireReaction {
		// Background monitoring agent continuously polls HTTP API for reactions
		// No need to call ExecuteAgents() here - monitoring runs in separate goroutine

		// Check state for user reaction (populated by MonitorAgent via HTTP API)
		if userReaction, exists := r.GetState("user_reaction"); exists {
			// Check if it's a non-empty reaction
			if reactionMap, ok := userReaction.(map[string]any); ok && len(reactionMap) > 0 {
				if _, ok := reactionMap["approved"].(bool); ok {
					r.processReaction(locals, userReaction)
					return
				}
			}
		}

		// Start background monitoring agent if not already running
		// (This covers the case where we restarted while in reaching phase)
		if locals.RequireReaction {
			r.startMonitoringIfNeeded()
		}

		// Check for timeout
		now := time.Now()
		if now.Unix()-locals.LastCheckTime.Unix() > int64(locals.TimeoutSeconds) {
			r.handleTimeout(locals)
			return
		}
	} else {
		// No reaction required, check if event time has passed
		if !locals.EventTime.IsZero() && time.Now().After(locals.EventTime) {
			r.StoreLog("📦 Event time passed, completing reminder")
			r.StoreStateMulti(map[string]any{
				"reminder_phase":           ReminderPhaseCompleted,
				"auto_completed":           true,
				"_reaction_packet_emitted": false,
				"achieving_percentage":     100,
			})
			locals.Phase = ReminderPhaseCompleted
			r.ProvideDone()
			return
		}

		// For reminders without reaction required and without explicit event_time,
		// complete after 10 seconds of reaching
		if now := time.Now(); now.After(locals.LastCheckTime.Add(10 * time.Second)) {
			r.StoreLog("📦 Completing reminder (no reaction required)")
			r.StoreStateMulti(map[string]any{
				"reminder_phase":           ReminderPhaseCompleted,
				"auto_completed":           true,
				"_reaction_packet_emitted": false,
				"achieving_percentage":     100,
			})
			locals.Phase = ReminderPhaseCompleted
			r.ProvideDone()
		}
	}
}

// processReaction processes user reaction
func (r *ReminderWant) processReaction(locals *ReminderLocals, reactionData any) {
	// Handle reaction data
	if reactionMap, ok := reactionData.(map[string]any); ok {
		if approved, ok := reactionMap["approved"].(bool); ok {
			if approved {
				r.SetStatus(WantStatusReaching) // Clear WaitingUserAction
				r.StoreLog("📦 Reminder approved by user")
				r.StoreStateMulti(map[string]any{
					"reminder_phase":           ReminderPhaseCompleted,
					"reaction_result":          "approved",
					"_reaction_packet_emitted": false,
					"achieving_percentage":     100,
				})
				locals.Phase = ReminderPhaseCompleted
				r.ProvideDone()

				// Trigger agent to delete queue immediately
				r.ExecuteAgents()
			} else {
				r.SetStatus(WantStatusReaching) // Clear WaitingUserAction
				r.StoreLog("📦 Reminder rejected by user")
				r.StoreStateMulti(map[string]any{
					"reminder_phase":           ReminderPhaseFailed,
					"reaction_result":          "rejected",
					"_reaction_packet_emitted": false,
					"achieving_percentage":     100,
				})
				locals.Phase = ReminderPhaseFailed

				// Trigger agent to delete queue immediately
				r.ExecuteAgents()
			}
		}
	}
}

// handleTimeout handles reaction timeout
func (r *ReminderWant) handleTimeout(locals *ReminderLocals) {
	if locals.RequireReaction {
		r.SetStatus(WantStatusReaching) // Clear WaitingUserAction before transitioning
		r.StoreLog("📦 Reaction timeout - marking as failed")
		r.StoreStateMulti(map[string]any{
			"reminder_phase":           ReminderPhaseFailed,
			"timeout":                  true,
			"_reaction_packet_emitted": false,
			"achieving_percentage":     100,
		})
		locals.Phase = ReminderPhaseFailed

		// Trigger agent to delete queue immediately
		r.ExecuteAgents()
	} else {
		r.StoreLog("📦 Reaction timeout - auto-completing")
		r.StoreStateMulti(map[string]any{
			"reminder_phase":           ReminderPhaseCompleted,
			"auto_completed":           true,
			"_reaction_packet_emitted": false,
			"achieving_percentage":     100,
		})
		locals.Phase = ReminderPhaseCompleted
		r.ProvideDone()

		// Trigger agent to delete queue immediately (if any existed)
		r.ExecuteAgents()
	}
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
