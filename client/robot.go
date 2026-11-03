package client

// robot.go — the robot's Plans and Acts (/api/v1/plans, /api/v1/acts), as
// want-spec's types.

import (
	"net/url"
	"strconv"

	ws "github.com/onelittlenightmusic/want-spec"
)

// CreatePlan declares a plan; the server names it.
func (c *Client) CreatePlan(p *ws.Plan) (*ws.Plan, error) {
	var out ws.Plan
	if err := c.Request("POST", "/api/v1/plans", p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPlans is the plans, newest first; limit 0 is all the server keeps,
// selector ("k=v,k2!=v2,k3") narrows them by their labels.
func (c *Client) ListPlans(limit int, selector string) ([]ws.Plan, error) {
	var out ws.PlanList
	if err := c.Request("GET", "/api/v1/plans"+listQuery(limit, selector), nil, &out); err != nil {
		return nil, err
	}
	return out.Plans, nil
}

// GetPlan is one plan.
func (c *Client) GetPlan(name string) (*ws.Plan, error) {
	var out ws.Plan
	if err := c.Request("GET", "/api/v1/plans/"+url.PathEscape(name), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePlan lets a plan go.
func (c *Client) DeletePlan(name string) error {
	return c.Request("DELETE", "/api/v1/plans/"+url.PathEscape(name), nil, nil)
}

// CreateAct carries out the plan the act names; answered at once, Pending.
func (c *Client) CreateAct(a *ws.Act) (*ws.Act, error) {
	var out ws.Act
	if err := c.Request("POST", "/api/v1/acts", a, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListActs is the acts, newest first; limit 0 is all the server keeps,
// selector ("k=v,k2!=v2,k3") narrows them by their labels.
func (c *Client) ListActs(limit int, selector string) ([]ws.Act, error) {
	var out ws.ActList
	if err := c.Request("GET", "/api/v1/acts"+listQuery(limit, selector), nil, &out); err != nil {
		return nil, err
	}
	return out.Acts, nil
}

// GetAct is one act as far as it has got. waitSeconds > 0 holds the answer
// until it is done or the seconds are up (the server holds 120 at most).
func (c *Client) GetAct(name string, waitSeconds int) (*ws.Act, error) {
	path := "/api/v1/acts/" + url.PathEscape(name)
	if waitSeconds > 0 {
		path += "?wait=" + strconv.Itoa(waitSeconds)
	}
	var out ws.Act
	if err := c.Request("GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteAct lets an act's record go; what it did on the board stays done.
func (c *Client) DeleteAct(name string) error {
	return c.Request("DELETE", "/api/v1/acts/"+url.PathEscape(name), nil, nil)
}

func listQuery(limit int, selector string) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if selector != "" {
		q.Set("labelSelector", selector)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}
