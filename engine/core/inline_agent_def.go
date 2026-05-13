package mywant

import want_spec "github.com/onelittlenightmusic/want-spec"

// InlineAgentDef defines an executable agent with inline script embedded in a YAML want type definition.
type InlineAgentDef = want_spec.InlineAgentDef

// ConditionDef defines a single declarative state condition (field operator value).
type ConditionDef = want_spec.ConditionDef

// AchievedWhenDef is an alias kept for backward compatibility. Use ConditionDef directly.
type AchievedWhenDef = want_spec.ConditionDef

// FinalizeWhen groups the conditions that determine how a ScriptableWant terminates.
type FinalizeWhen = want_spec.FinalizeWhen

// LifecycleHookDef defines actions executed at a want lifecycle event (onInitialize, onDelete, onAchieved).
type LifecycleHookDef = want_spec.LifecycleHookDef
