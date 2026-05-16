package types

import (
	"fmt"
	. "mywant/engine/core"
	"strings"
	"time"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[ReminderWant, ReminderLocals]("reminder")
	})
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		min := int(d.Minutes())
		sec := int(d.Seconds()) % 60
		if sec == 0 {
			return fmt.Sprintf("%dm", min)
		}
		return fmt.Sprintf("%dm%ds", min, sec)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if m == 0 && s == 0 {
		return fmt.Sprintf("%dh", h)
	}
	if s == 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dh%dm%ds", h, m, s)
}

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

	// Internal State fields (auto-synced)
	TimeoutSeconds        int    `mywant:"internal,_timeout_seconds"`
	ReactionQueueId       string `mywant:"internal,_local_rqid"`
	ReactionPacketEmitted bool   `mywant:"internal,_reaction_packet_emitted"`
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

	if err := r.StopAllBackgroundAgents(); err != nil {
		r.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := r.GetLocals()
	r.SetStatus(WantStatusIdle)
	locals.LastCheckTime = time.Now()
	locals.TimeoutSeconds = 300 // 5 minutes default timeout

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
	} else if r.hasWhenSpec() {
		// Scheduled via spec.when (e.g. fromGlobalParam or direct every/at).
		// Fire immediately on each restart triggered by the scheduler.
		eventTime = time.Now()
		locals.DurationFromNow = "0 seconds"
	} else {
		r.SetConfigError("timing", "Either 'event_time', 'duration_from_now', or 'when' spec must be provided")
		return
	}

	if !eventTime.IsZero() {
		reachingTime := eventTime.Add(-aheadDuration)
		locals.ReachingTime = reachingTime
		locals.EventTime = eventTime
		if eventTime.Before(time.Now()) || reachingTime.Before(time.Now()) {
			r.SetStatus(WantStatusReaching)
		}
	}

	locals.RequireReaction = r.GetBoolParam("require_reaction", false)
	locals.ReactionType = r.GetStringParam("reaction_type", "internal")

	r.SetGoal("message", locals.Message)
	r.SetGoal("ahead", locals.Ahead)
	r.SetGoal("require_reaction", locals.RequireReaction)
	r.SetGoal("reaction_type", locals.ReactionType)
	locals.ReactionQueueId = ""
	locals.ReactionPacketEmitted = false
	r.SetCurrent("user_reaction", map[string]any{})
	r.SetCurrent("reaction_result", "")

	if locals.DurationFromNow != "" {
		r.SetGoal("duration_from_now", locals.DurationFromNow)
	}

	if !locals.EventTime.IsZero() {
		r.SetGoal("event_time", locals.EventTime.Format(time.RFC3339))
	}

	if locals.RequireReaction {
		if err := r.ExecuteAgents(); err != nil {
			r.StoreLog("ERROR: Failed to execute agents: %v", err)
			r.SetStatus(WantStatusModuleError)
			r.SetCurrent("error_message", fmt.Sprintf("Agent execution failed: %v", err))
			return
		}
		r.startMonitoringIfNeeded()
	}

	r.SetStatus(WantStatusIdle)
}

func (r *ReminderWant) startMonitoringIfNeeded() {
	locals := r.GetLocals()
	if locals == nil || !locals.RequireReaction {
		return
	}

	reactionQueueId := locals.ReactionQueueId
	if reactionQueueId == "" {
		reactionQueueId = GetCurrent(r, "reaction_queue_id", "")
	}

	if reactionQueueId != "" {
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
	return r.GetStatus() == WantStatusAchieved
}

func (r *ReminderWant) CalculateAchievingPercentage() int {
	if r.IsAchieved() || r.Status == WantStatusAchieved || r.Status == WantStatusFailed {
		return 100
	}
	status := r.GetStatus()
	switch status {
	case WantStatusIdle:
		return 10
	case WantStatusReaching, WantStatusWaitingUserAction:
		return 50
	case WantStatusAchieved, WantStatusFailed:
		return 100
	default:
		return 0
	}
}

func (r *ReminderWant) Progress() {
	locals := r.GetLocals()
	r.SetCurrent("achieving_percentage", r.CalculateAchievingPercentage())
	timeRemaining := r.calculateTimeRemaining(locals)
	r.SetCurrent("time_remaining", timeRemaining)

	r.startMonitoringIfNeeded()

	status := r.GetStatus()
	reactionMap := GetCurrent(r, "user_reaction", map[string]any(nil))
	if len(reactionMap) > 0 {
		if status != WantStatusAchieved && status != WantStatusFailed {
			r.processReaction(locals, reactionMap)
		}
	}

	switch status {
	case WantStatusIdle:
		r.handleStatusIdle(locals)
	case WantStatusReaching, WantStatusWaitingUserAction:
		r.handleStatusReaching(locals)
	case WantStatusAchieved, WantStatusFailed:
		break
	default:
		r.SetModuleError("Status", fmt.Sprintf("Unknown status: %s", status))
	}
}

func (r *ReminderWant) calculateTimeRemaining(locals *ReminderLocals) string {
	now := time.Now()
	status := r.GetStatus()
	switch status {
	case WantStatusIdle:
		if !locals.ReachingTime.IsZero() {
			duration := locals.ReachingTime.Sub(now)
			if duration < 0 {
				return "0s"
			}
			return formatDuration(duration)
		}
	case WantStatusReaching, WantStatusWaitingUserAction:
		if !locals.EventTime.IsZero() {
			duration := locals.EventTime.Sub(now)
			if duration < 0 {
				return "0s"
			}
			return formatDuration(duration)
		}
	case WantStatusAchieved, WantStatusFailed:
		return "0s"
	}
	return ""
}

func (r *ReminderWant) handleStatusIdle(locals *ReminderLocals) {
	if locals.ReachingTime.IsZero() {
		return
	}
	if time.Now().After(locals.ReachingTime) {
		r.SetStatus(WantStatusReaching)
		r.SetCurrent("user_reaction", map[string]any{})
		r.SetCurrent("reaction_result", "")
		r.ExecuteAgents()
	}
}

func (r *ReminderWant) emitReactionPacketIfNeeded(locals *ReminderLocals) {
	if !locals.RequireReaction || locals.ReactionPacketEmitted {
		return
	}
	if locals.ReactionQueueId == "" {
		return
	}
	out := NewDataObject("reaction_request")
	out.Set("reaction_id", locals.ReactionQueueId)
	out.Set("reaction_type", locals.ReactionType)
	out.Set("source_want", r.Metadata.Name)
	r.Provide(out)
	locals.ReactionPacketEmitted = true
}

func (r *ReminderWant) handleStatusReaching(locals *ReminderLocals) {
	queueId := GetCurrent(r, "reaction_queue_id", "")
	if queueId != "" {
		locals.ReactionQueueId = queueId
	}
	r.emitReactionPacketIfNeeded(locals)
	if locals.RequireReaction && r.GetStatus() != WantStatusWaitingUserAction {
		r.SetStatus(WantStatusWaitingUserAction)
	}

	if locals.RequireReaction {
		reactionMap := GetCurrent(r, "user_reaction", map[string]any(nil))
		if len(reactionMap) > 0 {
			if _, ok := reactionMap["approved"].(bool); ok {
				r.processReaction(locals, reactionMap)
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
	r.SetStatus(WantStatusAchieved)
	r.SetCurrent("auto_completed", true)
	locals.ReactionPacketEmitted = false
	r.SetCurrent("achieving_percentage", 100)
	r.ProvideDone()
}

func (r *ReminderWant) processReaction(locals *ReminderLocals, reactionMap map[string]any) {
	if approved, ok := reactionMap["approved"].(bool); ok {
		status := WantStatusReaching
		result := ""
		if approved {
			r.StoreLog("📦 Reminder approved by user")
			status = WantStatusAchieved
			result = "approved"
		} else {
			r.StoreLog("📦 Reminder rejected by user")
			status = WantStatusFailed
			result = "rejected"
		}
		r.SetStatus(status)
		r.SetCurrent("reaction_result", result)
		r.SetCurrent("user_reaction", reactionMap)
		locals.ReactionPacketEmitted = false
		r.SetCurrent("achieving_percentage", 100)
		if status == WantStatusAchieved {
			r.ProvideDone()
		}
		r.ExecuteAgents()
	}
}

func (r *ReminderWant) handleTimeout(locals *ReminderLocals) {
	r.SetStatus(WantStatusReaching)
	r.StoreLog("📦 Reaction timeout")
	r.SetCurrent("timeout", true)
	status := WantStatusIdle
	result := ""
	if locals.RequireReaction {
		status = WantStatusFailed
		result = "timeout"
	} else {
		status = WantStatusAchieved
		r.SetCurrent("auto_completed", true)
		r.ProvideDone()
	}
	r.SetStatus(status)
	r.SetCurrent("reaction_result", result)
	locals.ReactionPacketEmitted = false
	r.SetCurrent("achieving_percentage", 100)
	r.ExecuteAgents()
}

func parseDurationString(s string) (time.Duration, error) {
	var unit time.Duration
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}
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
	var num int
	_, err := fmt.Sscanf(numStr, "%d", &num)
	if err != nil {
		return 0, fmt.Errorf("invalid number in duration: %s", numStr)
	}
	return time.Duration(num) * unit, nil
}
