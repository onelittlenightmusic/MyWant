package mywant

import (
	"fmt"
	"strings"
)

func (cb *ChainBuilder) processAutoConnections() {

	// Collect all wants with RecipeAgent enabled
	autoConnectWants := make([]*runtimeWant, 0)
	allWants := make([]*runtimeWant, 0)

	for _, runtimeWant := range cb.wants {
		allWants = append(allWants, runtimeWant)
		want := runtimeWant.want
		if cb.hasRecipeAgent(want) {
			autoConnectWants = append(autoConnectWants, runtimeWant)
		}
	}
	for _, runtimeWant := range autoConnectWants {
		want := runtimeWant.want
		cb.autoConnectWant(want, allWants)
		// Note: want object itself has been updated, no need to sync to separate spec copy
	}

}

// hasRecipeAgent checks if a want has RecipeAgent functionality enabled
func (cb *ChainBuilder) hasRecipeAgent(want *Want) bool {
	labels := want.GetLabels()
	if len(labels) > 0 {
		if role, ok := labels["role"]; ok && role == "coordinator" {
			return true
		}
	}
	if want.Metadata.Type == "level1_coordinator" || want.Metadata.Type == "level2_coordinator" {
		return true
	}

	return false
}

// autoConnectWant connects a RecipeAgent want to all compatible wants with matching approval_id
func (cb *ChainBuilder) autoConnectWant(want *Want, allWants []*runtimeWant) {
	approvalID := cb.extractApprovalID(want)
	if approvalID == "" {
		return
	}

	if want.Spec.Using == nil {
		want.Spec.Using = make([]map[string]string, 0)
	}

	for _, otherRuntimeWant := range allWants {
		cb.tryAutoConnectToWant(want, otherRuntimeWant.want)
	}
}

// extractApprovalID extracts approval_id from want params or labels
func (cb *ChainBuilder) extractApprovalID(want *Want) string {
	// Try params first
	if want.Spec.Params != nil {
		approvalID := ExtractMapString(want.Spec.Params, "approval_id")
		if approvalID != "" {
			return approvalID
		}
	}

	// Fall back to labels
	labels := want.GetLabels()
	if len(labels) > 0 {
		if approvalID, ok := labels["approval_id"]; ok && approvalID != "" {
			return approvalID
		}
	}

	return ""
}

// tryAutoConnectToWant attempts to auto-connect a want to another want
func (cb *ChainBuilder) tryAutoConnectToWant(want *Want, otherWant *Want) {
	// Skip self
	if otherWant.Metadata.Name == want.Metadata.Name {
		return
	}

	approvalID := cb.extractApprovalID(want)
	otherApprovalID := cb.extractApprovalID(otherWant)

	// Must have matching approval_id
	if approvalID == "" || otherApprovalID != approvalID {
		return
	}

	// Must be a data provider want
	if !cb.isDataProviderWant(otherWant) {
		return
	}

	cb.addAutoConnection(want, otherWant)
}

// isDataProviderWant checks if a want is a data provider (evidence or description)
func (cb *ChainBuilder) isDataProviderWant(want *Want) bool {
	labels := want.GetLabels()
	if len(labels) == 0 {
		return false
	}

	role := labels["role"]
	return role == "evidence-provider" || role == "description-provider"
}

// addAutoConnection adds an auto-connection from otherWant to want if not duplicate
func (cb *ChainBuilder) addAutoConnection(want *Want, otherWant *Want) {
	connectionKey := cb.generateConnectionKey(want)
	cb.addConnectionLabel(otherWant, want)

	selector := cb.buildConnectionSelector(want, otherWant, connectionKey)

	// Check for duplicate selector
	if cb.hasDuplicateSelector(want, selector) {
		return
	}

	want.Spec.Using = append(want.Spec.Using, selector)
}

// buildConnectionSelector builds a connection selector map
func (cb *ChainBuilder) buildConnectionSelector(want *Want, otherWant *Want, connectionKey string) map[string]string {
	selector := make(map[string]string)

	if connectionKey != "" {
		labelKey := fmt.Sprintf("used_by_%s", connectionKey)
		selector[labelKey] = want.Metadata.Name
	} else {
		// Fallback to role-based selector
		labels := otherWant.GetLabels()
		role := labels["role"]
		selector["role"] = role
	}

	return selector
}

// hasDuplicateSelector checks if a selector already exists in want's using list
func (cb *ChainBuilder) hasDuplicateSelector(want *Want, selector map[string]string) bool {
	for _, existingSelector := range want.Spec.Using {
		if cb.selectorsMatch(existingSelector, selector) {
			return true
		}
	}
	return false
}

// selectorsMatch checks if two selectors are equal
func (cb *ChainBuilder) selectorsMatch(selector1, selector2 map[string]string) bool {
	if len(selector1) != len(selector2) {
		return false
	}

	for k, v := range selector2 {
		if selector1[k] != v {
			return false
		}
	}

	return true
}

func (cb *ChainBuilder) addConnectionLabel(sourceWant *Want, consumerWant *Want) {
	sourceWant.metadataMutex.Lock()
	if sourceWant.Metadata.Labels == nil {
		sourceWant.Metadata.Labels = make(map[string]string)
	}

	// Generate unique connection label based on consumer want Extract meaningful identifier from consumer (e.g., level1, level2, etc.)
	connectionKey := cb.generateConnectionKey(consumerWant)

	if connectionKey != "" {
		labelKey := fmt.Sprintf("used_by_%s", connectionKey)
		sourceWant.Metadata.Labels[labelKey] = consumerWant.Metadata.Name
	}
	sourceWant.metadataMutex.Unlock()
}

// generateConnectionKey creates a unique key based on consumer want characteristics
func (cb *ChainBuilder) generateConnectionKey(consumerWant *Want) string {
	// Try to extract meaningful identifier from labels first
	labels := consumerWant.GetLabels()
	if len(labels) > 0 {
		if level, ok := labels["approval_level"]; ok {
			return fmt.Sprintf("level%s", level)
		}
		if component, ok := labels["component"]; ok {
			return component
		}
		if category, ok := labels["category"]; ok {
			return category
		}
	}

	// Extract from want type as fallback
	wantType := consumerWant.Metadata.Type
	if strings.Contains(wantType, "level1") {
		return "level1"
	}
	if strings.Contains(wantType, "level2") {
		return "level2"
	}
	if strings.Contains(wantType, "coordinator") {
		return "coordinator"
	}

	// Use sanitized want name as last resort
	return strings.ReplaceAll(consumerWant.Metadata.Name, "-", "_")
}
