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
	LastCheckTime   time.Time

	// State fields (auto-synced)
	Phase                 string         `mywant:"current,reminder_phase"`
	TimeoutSeconds        int            `mywant:"current,timeout"`
	ReactionQueueId       string         `mywant:"current,reaction_queue_id"`
	UserReaction          map[string]any `mywant:"current,user_reaction"`
	ReactionResult        string         `mywant:"current,reaction_result"`
	AutoCompleted         bool           `mywant:"current,auto_completed"`
	ReactionPacketEmitted bool           `mywant:"internal,_reaction_packet_emitted"`
	ReachingTimeStr       string         `mywant:"internal,reaching_time"`
	ErrorMessage          string         `mywant:"current,error_message"`
	TimeRemaining         string         `mywant:"current,time_remaining"`
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
	if err := r.StopAllBackgroundAgents(); err != nil {
		r.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	// Get locals (guaranteed to be initialized by framework)
	locals := r.GetLocals()
	locals.Phase = ReminderPhaseWaiting
	locals.LastCheckTime = time.Now()
	locals.TimeoutSeconds = 300 // 5 minutes default timeout

	// Parse and validate parameters using ConfigError pattern
	locals.Message = r.GetStringParam("message", "")
	if locals.Message == "" {
		r.SetConfigError("message", "Missing required parameter 'message'")
		return
	}

	locals.Ahead = r.GetStringParam("ahead", "5 minutes")
	aheadDuration, err := parseDurationString(locals.Ahead)
	if err != nil {
		r.SetConfigError("ahead", fmt.Sprintf("Invalid ahead parameter '%s': %v", locals.Ahead, err))
		return
	}

	eventTimeStr := r.GetStringParam("event_time", "")
	durationFromNowStr := r.GetStringParam("duration_from_now", "")

	if eventTimeStr != "" && durationFromNowStr != "" {
		r.SetConfigError("event_time/duration_from_now", "Cannot provide both 'event_time' and 'duration_from_now'")
		return
	}

	var eventTime time.Time
	if eventTimeStr != "" {
		var parseErr error
		eventTime, parseErr = time.Parse(time.RFC3339, eventTimeStr)
		if parseErr != nil {
			r.SetConfigError("event_time", fmt.Sprintf("Invalid event_time format (use RFC3339): %v", parseErr))
			return
		}
	} else if durationFromNowStr != "" {
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

	if !eventTime.IsZero() {
		reachingTime := eventTime.Add(-aheadDuration)
		locals.ReachingTime = reachingTime
		locals.EventTime = eventTime
		locals.ReachingTimeStr = reachingTime.Format(time.RFC3339)

		if eventTime.Before(time.Now()) {
			locals.Phase = ReminderPhaseReaching
		}
	}

	locals.RequireReaction = r.GetBoolParam("require_reaction", false)
	locals.ReactionType = r.GetStringParam("reaction_type", "internal")

	// Store initial state
	r.SetGoal("message", locals.Message)
	r.SetGoal("ahead", locals.Ahead)
	r.SetGoal("require_reaction", locals.RequireReaction)
	r.SetGoal("reaction_type", locals.ReactionType)
	locals.ReactionQueueId = ""
	locals.ReactionPacketEmitted = false
	locals.UserReaction = nil
	locals.ReactionResult = ""

	if locals.DurationFromNow != "" {
		r.SetGoal("duration_from_now", locals.DurationFromNow)
	}

	if !locals.EventTime.IsZero() {
		r.SetGoal("event_time", locals.EventTime.Format(time.RFC3339))
	}

	if locals.RequireReaction {
		if err := r.ExecuteAgents(); err != nil {
			r.StoreLog("ERROR: Failed to execute agents: %v", err)
			locals.Phase = ReminderPhaseFailed
			locals.ErrorMessage = fmt.Sprintf("Agent execution failed: %v", err)
			return
		}
		r.startMonitoringIfNeeded()
	}
}

// startMonitoringIfNeeded starts the reaction monitor agent if not already running
func (r *ReminderWant) startMonitoringIfNeeded() {
	locals := r.GetLocals()
	if locals == nil || !locals.RequireReaction {
		return
	}

	if locals.ReactionQueueId != "" {
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

func (r *ReminderWant) hasWhenSpec() bool {
	return r.Spec.When != nil && len(r.Spec.When) > 0
}

func (r *ReminderWant) IsAchieved() bool {
	return r.GetLocals().Phase == ReminderPhaseCompleted
}

func (r *ReminderWant) CalculateAchievingPercentage() int {
	if r.IsAchieved() || r.Status == WantStatusAchieved || r.Status == WantStatusFailed {
		return 100
	}
	locals := r.GetLocals()
	switch locals.Phase {
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

func (r *ReminderWant) Progress() {
	locals := r.GetLocals()
	r.SetPredefined("achieving_percentage", r.CalculateAchievingPercentage())
	locals.TimeRemaining = r.calculateTimeRemaining(locals)

	switch locals.Phase {
	case ReminderPhaseWaiting:
		r.handlePhaseWaiting(locals)
	case ReminderPhaseReaching:
		r.handlePhaseReaching(locals)
	case ReminderPhaseCompleted, ReminderPhaseFailed:
		break
	default:
		r.SetModuleError("Phase", fmt.Sprintf("Unknown phase: %s", locals.Phase))
	}
}

func (r *ReminderWant) calculateTimeRemaining(locals *ReminderLocals) string {
	now := time.Now()
	switch locals.Phase {
	case ReminderPhaseWaiting:
		if !locals.ReachingTime.IsZero() {
			duration := locals.ReachingTime.Sub(now)
			if duration < 0 { return "0s" }
			return formatDuration(duration)
		}
	case ReminderPhaseReaching:
		if !locals.EventTime.IsZero() {
			duration := locals.EventTime.Sub(now)
			if duration < 0 { return "0s" }
			return formatDuration(duration)
		}
	case ReminderPhaseCompleted, ReminderPhaseFailed:
		return "0s"
	}
	return ""
}

func formatDuration(d time.Duration) string {
	if d < 0 { return "0s" }
	d = d.Round(time.Second)
	if d < time.Minute { return fmt.Sprintf("%ds", int(d.Seconds())) }
	if d < time.Hour {
		min := int(d.Minutes())
		sec := int(d.Seconds()) % 60
		if sec == 0 { return fmt.Sprintf("%dm", min) }
		return fmt.Sprintf("%dm%ds", min, sec)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if m == 0 && s == 0 { return fmt.Sprintf("%dh", h) }
	if s == 0 { return fmt.Sprintf("%dh%dm", h, m) }
	return fmt.Sprintf("%dh%dm%ds", h, m, s)
}

func (r *ReminderWant) handlePhaseWaiting(locals *ReminderLocals) {
	if locals.ReachingTime.IsZero() { return }
	if time.Now().After(locals.ReachingTime) {
		locals.Phase = ReminderPhaseReaching
		locals.ReactionQueueId = ""
		locals.ReactionPacketEmitted = false
		locals.UserReaction = nil
		locals.ReactionResult = ""
		r.ExecuteAgents()
	}
}

func (r *ReminderWant) emitReactionPacketIfNeeded(locals *ReminderLocals) {
	if !locals.RequireReaction || locals.ReactionPacketEmitted { return }
	if locals.ReactionQueueId != "" {
		r.Provide(map[string]any{
			"reaction_id":   locals.ReactionQueueId,
			"reaction_type": locals.ReactionType,
			"source_want":   r.Metadata.Name,
		})
		locals.ReactionPacketEmitted = true
	}
}

func (r *ReminderWant) handlePhaseReaching(locals *ReminderLocals) {
	r.emitReactionPacketIfNeeded(locals)
	if locals.RequireReaction && r.GetStatus() != WantStatusWaitingUserAction {
		r.SetStatus(WantStatusWaitingUserAction)
	}

	if locals.RequireReaction {
		if len(locals.UserReaction) > 0 {
			if _, ok := locals.UserReaction["approved"].(bool); ok {
				r.processReaction(locals, locals.UserReaction)
				return
			}
		}
		r.startMonitoringIfNeeded()
		if time.Now().Unix()-locals.LastCheckTime.Unix() > int64(locals.TimeoutSeconds) {
			r.handleTimeout(locals)
		}
	} else {
		if !locals.EventTime.IsZero() && time.Now().After(locals.EventTime) {
			r.completeReminder(locals, "📦 Event time passed, completing reminder")
		} else if time.Now().After(locals.LastCheckTime.Add(10 * time.Second)) {
			r.completeReminder(locals, "📦 Completing reminder (no reaction required)")
		}
	}
}

func (r *ReminderWant) completeReminder(locals *ReminderLocals, logMsg string) {
	r.StoreLog("%s", logMsg)
	locals.Phase = ReminderPhaseCompleted
	locals.AutoCompleted = true
	locals.ReactionPacketEmitted = false
	r.SetPredefined("achieving_percentage", 100)
	r.ProvideDone()
}

func (r *ReminderWant) processReaction(locals *ReminderLocals, reactionMap map[string]any) {
	if approved, ok := reactionMap["approved"].(bool); ok {
		r.SetStatus(WantStatusReaching)
		if approved {
			r.StoreLog("📦 Reminder approved by user")
			locals.Phase = ReminderPhaseCompleted
			locals.ReactionResult = "approved"
		} else {
			r.StoreLog("📦 Reminder rejected by user")
			locals.Phase = ReminderPhaseFailed
			locals.ReactionResult = "rejected"
		}
		locals.ReactionPacketEmitted = false
		r.SetPredefined("achieving_percentage", 100)
		if locals.Phase == ReminderPhaseCompleted { r.ProvideDone() }
		r.ExecuteAgents()
	}
}

func (r *ReminderWant) handleTimeout(locals *ReminderLocals) {
	r.SetStatus(WantStatusReaching)
	r.StoreLog("📦 Reaction timeout")
	if locals.RequireReaction {
		locals.Phase = ReminderPhaseFailed
		locals.ReactionResult = "timeout"
	} else {
		locals.Phase = ReminderPhaseCompleted
		locals.AutoCompleted = true
		r.ProvideDone()
	}
	locals.ReactionPacketEmitted = false
	r.SetPredefined("achieving_percentage", 100)
	r.ExecuteAgents()
}

func parseDurationString(s string) (time.Duration, error) {
	var unit time.Duration
	if len(s) < 2 { return 0, fmt.Errorf("invalid duration format: %s", s) }
	var numStr string
	for i, c := range s {
		if c >= '0' && c <= '9' { numStr += string(c) } else {
			s = s[i:]
			break
		}
	}
	if numStr == "" { return 0, fmt.Errorf("no number found in duration: %s", s) }
	s = strings.TrimSpace(s)
	switch s {
	case "second", "seconds": unit = time.Second
	case "minute", "minutes": unit = time.Minute
	case "hour", "hours": unit = time.Hour
	case "day", "days": unit = 24 * time.Hour
	default: return 0, fmt.Errorf("unknown duration unit: %s", s)
	}
	var num int
	_, err := fmt.Sscanf(numStr, "%d", &num)
	if err != nil { return 0, fmt.Errorf("invalid number in duration: %s", numStr) }
	return time.Duration(num) * unit, nil
}
