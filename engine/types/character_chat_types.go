package types

import (
	. "mywant/engine/core"
)

func init() {
	RegisterWithInit(func() {
		RegisterWantImplementation[CharacterChatWant, CharacterChatLocals]("character_chat")
	})
}

// CharacterChatLocals holds this want's own bookkeeping. Only the webhook's,
// because that is all this want does.
type CharacterChatLocals struct {
	WebhookLocals
}

// chatWebhookConfig mirrors ccWebhookConfig's state keys on purpose.
//
// The card that draws a chat — the message list, the input box, the send button
// — already exists and reads cc_messages. Naming these fields anything else
// would have meant a second card that looked the same and shared no code, so
// the fields keep the names the card already asks for. What differs is what is
// behind them, which is nothing.
var chatWebhookConfig = WebhookWantConfig{
	StatePrefix:      "cc",
	MonitorAgentName: "monitor_cc_webhook",
	LogPrefix:        "[CHARACTER-CHAT]",
	SecretParamName:  "webhook_secret",
}

// CharacterChatWant is a character's voice and mailbox: a chat window with
// nothing behind it.
//
// The robot is the exception on this board, not the rule. It has a coding agent
// inside it, which is why talking to it produces an answer — but a character is
// a person, and a person's want has nobody to ask. So this want type is the
// robot's arrangement with the agent taken out: the same webhook in, the same
// message history, the same card, and no LLM, no session, no CLI, no cost.
//
// What arrives is what the character says. Nothing here decides that or
// transforms it; the message is put where the interface can find it, and the
// board draws it over whoever it belongs to.
type CharacterChatWant struct {
	Want
}

func (w *CharacterChatWant) GetLocals() *CharacterChatLocals {
	return CheckLocalsInitialized[CharacterChatLocals](&w.Want)
}

func (w *CharacterChatWant) Initialize() {
	w.StoreLog("[CHARACTER-CHAT] Initializing: %s", w.Metadata.Name)

	if err := w.StopAllBackgroundAgents(); err != nil {
		w.StoreLog("ERROR: Failed to stop existing background agents: %v", err)
	}

	locals := w.GetLocals()
	w.SetCurrent("interactive", true)
	InitializeWebhook(&w.Want, chatWebhookConfig, &locals.WebhookLocals)
}

// Progress does nothing on purpose.
//
// The webhook handler writes the message straight into state, and there is no
// request to send, no response to wait for and no phase to advance — the
// machinery the robot needs for those is exactly the machinery this type exists
// without. Kept as a method rather than omitted because the engine ticks every
// want, and a tick that has nothing to do should say so here rather than
// somewhere else deciding this type is special.
func (w *CharacterChatWant) Progress() {
	w.SetCurrent("achieving_percentage", 50)
}

// IsAchieved is always false: a person does not finish being available to talk
// to. Same as the robot, and for the same reason.
func (w *CharacterChatWant) IsAchieved() bool { return false }
