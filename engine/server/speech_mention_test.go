package server

import "testing"

// Addressing the robot has to be deliberate, and it has to be the only thing
// that reaches the agent — everything else said is just said.

func TestMentionsRobot(t *testing.T) {
	addressed := []string{
		"@robot 直して",
		"これ直せる? @robot",
		"@ROBOT fix this", // however it was typed
		"@robot",          // an address with nothing asked is still an address
		"ねえ@robot、これ",     // no space needed around it in Japanese
	}
	for _, text := range addressed {
		if !mentionsRobot(text) {
			t.Errorf("%q does not address the robot, but should", text)
		}
	}

	// A word that merely starts the same way is a subject, not a summons.
	ordinary := []string{
		"ロボットについて話そう",
		"@robotics is a field",
		"@robot_helper に頼んだ",
		"email me at a@robots.example",
		"",
	}
	for _, text := range ordinary {
		if mentionsRobot(text) {
			t.Errorf("%q addresses the robot, but should not", text)
		}
	}
}

// The mention is how the message was routed, not part of what was asked.
func TestStripRobotMention(t *testing.T) {
	cases := map[string]string{
		"@robot 直して":        "直して",
		"これ直せる? @robot":     "これ直せる?",
		"@robot":            "",
		"@ROBOT fix this":   "fix this",
		"ねえ @robot 、これ":     "ねえ 、これ",
		"@robotics is fine": "@robotics is fine", // not a mention, so untouched
	}
	for input, want := range cases {
		if got := stripRobotMention(input); got != want {
			t.Errorf("stripRobotMention(%q) = %q, want %q", input, got, want)
		}
	}
}
