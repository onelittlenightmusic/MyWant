package client

import (
	"fmt"
)

// AnalyzeWantForRecipe analyzes a want and returns recipe creation recommendations
func (c *Client) AnalyzeWantForRecipe(wantID string) (*WantRecipeAnalysis, error) {
	var result WantRecipeAnalysis
	err := c.Request("GET", fmt.Sprintf("/api/v1/wants/%s/recipe-analysis", wantID), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// ListRecipes retrieves all recipes
func (c *Client) ListRecipes() (map[string]GenericRecipe, error) {
	var result map[string]GenericRecipe
	err := c.Request("GET", "/api/v1/recipes", nil, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetRecipe retrieves a recipe by ID
func (c *Client) GetRecipe(id string) (*GenericRecipe, error) {
	var result GenericRecipe
	err := c.Request("GET", fmt.Sprintf("/api/v1/recipes/%s", id), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateRecipe creates a new recipe
func (c *Client) CreateRecipe(recipe GenericRecipe) error {
	return c.Request("POST", "/api/v1/recipes", recipe, nil)
}

// SaveRecipeFromWant creates a recipe from an existing want
func (c *Client) SaveRecipeFromWant(req SaveRecipeFromWantRequest) (*SaveRecipeResponse, error) {
	var result SaveRecipeResponse
	err := c.Request("POST", "/api/v1/recipes/from-want", req, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteRecipe deletes a recipe
func (c *Client) DeleteRecipe(id string) error {
	return c.Request("DELETE", fmt.Sprintf("/api/v1/recipes/%s", id), nil, nil)
}
