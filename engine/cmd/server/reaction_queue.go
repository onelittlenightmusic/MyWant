package main

import (
	"mywant/engine/cmd/types"
)

// ReactionQueue is an alias to types.ReactionQueue for convenience in main package
type ReactionQueue = types.ReactionQueue

// ReactionData is an alias to types.ReactionData for convenience in main package
type ReactionData = types.ReactionData

// NewReactionQueue creates a new reaction queue
func NewReactionQueue() *ReactionQueue {
	return types.NewReactionQueue()
}
