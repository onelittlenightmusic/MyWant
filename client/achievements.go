package client

import (
	"fmt"
	"time"
)

// Achievement mirrors engine/core.Achievement for the CLI client.
type Achievement struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	AgentName         string         `json:"agentName"`
	WantID            string         `json:"wantID"`
	WantName          string         `json:"wantName"`
	Category          string         `json:"category"`
	Level             int            `json:"level"`
	EarnedAt          time.Time      `json:"earnedAt"`
	AwardedBy         string         `json:"awardedBy"`
	UnlocksCapability string         `json:"unlocksCapability,omitempty"`
	Unlocked          bool           `json:"unlocked"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type AchievementListResponse struct {
	Achievements []Achievement `json:"achievements"`
	Count        int           `json:"count"`
}

// ListAchievements retrieves all achievements.
func (c *Client) ListAchievements() (*AchievementListResponse, error) {
	var result AchievementListResponse
	if err := c.Request("GET", "/api/v1/achievements", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAchievement retrieves a single achievement by ID.
func (c *Client) GetAchievement(id string) (*Achievement, error) {
	var result Achievement
	if err := c.Request("GET", fmt.Sprintf("/api/v1/achievements/%s", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateAchievement creates a new achievement (manual/human award).
func (c *Client) CreateAchievement(a Achievement) (*Achievement, error) {
	var result Achievement
	if err := c.Request("POST", "/api/v1/achievements", a, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateAchievement replaces an achievement by ID.
func (c *Client) UpdateAchievement(id string, a Achievement) (*Achievement, error) {
	var result Achievement
	if err := c.Request("PUT", fmt.Sprintf("/api/v1/achievements/%s", id), a, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LockAchievement deactivates an achievement's capability (sets unlocked=false).
func (c *Client) LockAchievement(id string) (*Achievement, error) {
	var result Achievement
	if err := c.Request("PATCH", fmt.Sprintf("/api/v1/achievements/%s/lock", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UnlockAchievement activates an achievement's capability (sets unlocked=true).
func (c *Client) UnlockAchievement(id string) (*Achievement, error) {
	var result Achievement
	if err := c.Request("PATCH", fmt.Sprintf("/api/v1/achievements/%s/unlock", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteAchievement removes an achievement by ID.
func (c *Client) DeleteAchievement(id string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/achievements/%s", id), nil, nil)
}

// ── Rules ─────────────────────────────────────────────────────────────────────

// AchievementCondition mirrors engine/core.AchievementCondition.
type AchievementCondition struct {
	AgentCapability string `json:"agentCapability,omitempty"`
	WantType        string `json:"wantType,omitempty"`
	CompletedCount  int    `json:"completedCount"`
}

// AchievementAward mirrors engine/core.AchievementAward.
type AchievementAward struct {
	Title             string `json:"title"`
	Description       string `json:"description"`
	Level             int    `json:"level"`
	Category          string `json:"category"`
	UnlocksCapability string `json:"unlocksCapability,omitempty"`
}

// AchievementRule mirrors engine/core.AchievementRule.
type AchievementRule struct {
	ID        string               `json:"id"`
	Active    bool                 `json:"active"`
	Condition AchievementCondition `json:"condition"`
	Award     AchievementAward     `json:"award"`
}

type AchievementRuleListResponse struct {
	Rules []AchievementRule `json:"rules"`
	Count int               `json:"count"`
}

func (c *Client) ListAchievementRules() (*AchievementRuleListResponse, error) {
	var result AchievementRuleListResponse
	if err := c.Request("GET", "/api/v1/achievements/rules", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) GetAchievementRule(id string) (*AchievementRule, error) {
	var result AchievementRule
	if err := c.Request("GET", fmt.Sprintf("/api/v1/achievements/rules/%s", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) CreateAchievementRule(r AchievementRule) (*AchievementRule, error) {
	var result AchievementRule
	if err := c.Request("POST", "/api/v1/achievements/rules", r, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteAchievementRule(id string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/achievements/rules/%s", id), nil, nil)
}
