import React, { useState, useCallback, useMemo, forwardRef } from 'react';
import { Tag } from 'lucide-react';
import { CollapsibleFormSection } from '../CollapsibleFormSection';
import { LabelAutocomplete } from '../LabelAutocomplete';
import { ChipItem, SectionNavigationCallbacks } from '@/types/formSection';

/**
 * Props for LabelsSection
 */
interface LabelsSectionProps {
  /** Current labels as key-value pairs */
  labels: Record<string, string>;
  /** Callback when labels change */
  onChange: (labels: Record<string, string>) => void;
  /** Whether the section is collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
}

/**
 * Editing state for a label
 */
interface LabelDraft {
  key: string;
  value: string;
}

/**
 * LabelsSection - Adapter for labels using CollapsibleFormSection
 *
 * Manages labels as Record<string, string> internally and provides
 * a consistent UI using the generic CollapsibleFormSection component.
 */
export const LabelsSection = forwardRef<HTMLButtonElement, LabelsSectionProps>(({
  labels,
  onChange,
  isCollapsed,
  onToggleCollapse,
  navigationCallbacks,
}, ref) => {
  // Editing state
  const [editingLabelKey, setEditingLabelKey] = useState<string | null>(null);
  const [editingLabelDraft, setEditingLabelDraft] = useState<LabelDraft>({ key: '', value: '' });

  /**
   * Convert labels to chip items for display
   */
  const chipItems = useMemo((): ChipItem[] => {
    return Object.entries(labels)
      .filter(([key]) => key !== editingLabelKey) // Hide chip being edited
      .map(([key, value]) => ({
        key,
        display: `${key}: ${value}`,
      }));
  }, [labels, editingLabelKey]);

  /**
   * Start editing an existing label
   */
  const handleEditChip = useCallback((index: number) => {
    const entries = Object.entries(labels).filter(([key]) => key !== editingLabelKey);
    const [key, value] = entries[index];

    setEditingLabelKey(key);
    setEditingLabelDraft({ key, value });
  }, [labels, editingLabelKey]);

  /**
   * Remove a label
   */
  const handleRemoveChip = useCallback((index: number) => {
    const entries = Object.entries(labels).filter(([key]) => key !== editingLabelKey);
    const [keyToRemove] = entries[index];

    const newLabels = { ...labels };
    delete newLabels[keyToRemove];
    onChange(newLabels);
  }, [labels, editingLabelKey, onChange]);

  /**
   * Start adding a new label
   */
  const handleAddItem = useCallback(() => {
    setEditingLabelKey('__new__');
    setEditingLabelDraft({ key: '', value: '' });
  }, []);

  /**
   * Save the current label edit
   */
  const handleSave = useCallback(() => {
    // Only update if draft has a key value
    if (editingLabelDraft.key.trim()) {
      const newLabels = { ...labels };

      // Remove old key if editing existing label
      if (editingLabelKey !== '__new__' && editingLabelKey !== null) {
        delete newLabels[editingLabelKey];
      }

      // Add new key-value pair
      newLabels[editingLabelDraft.key] = editingLabelDraft.value;

      onChange(newLabels);
    }

    // Reset editing state
    setEditingLabelKey(null);
    setEditingLabelDraft({ key: '', value: '' });
  }, [editingLabelKey, editingLabelDraft, labels, onChange]);

  /**
   * Cancel the current label edit
   */
  const handleCancel = useCallback(() => {
    setEditingLabelKey(null);
    setEditingLabelDraft({ key: '', value: '' });
  }, []);

  /**
   * Handle escape key - same as cancel
   */
  const handleEscape = useCallback(() => {
    handleCancel();
  }, [handleCancel]);

  /**
   * Handle remove from edit form
   */
  const handleRemoveFromEditForm = useCallback(() => {
    if (editingLabelKey !== null && editingLabelKey !== '__new__') {
      const newLabels = { ...labels };
      delete newLabels[editingLabelKey];
      onChange(newLabels);
    }

    setEditingLabelKey(null);
    setEditingLabelDraft({ key: '', value: '' });
  }, [editingLabelKey, labels, onChange]);

  /**
   * Render the edit form
   */
  const renderEditForm = useCallback(() => (
    <div className="space-y-3">
      <LabelAutocomplete
        keyValue={editingLabelDraft.key}
        valueValue={editingLabelDraft.value}
        onKeyChange={(newKey) => setEditingLabelDraft(prev => ({ ...prev, key: newKey }))}
        onValueChange={(newValue) => setEditingLabelDraft(prev => ({ ...prev, value: newValue }))}
        onRemove={handleRemoveFromEditForm}
        onLeftKey={handleEscape}
      />
      <div className="flex gap-2">
        <button
          type="button"
          onClick={handleSave}
          className="px-3 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          Save
        </button>
        <button
          type="button"
          onClick={handleCancel}
          className="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded-md hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-gray-300"
        >
          Cancel
        </button>
      </div>
    </div>
  ), [editingLabelDraft, handleSave, handleCancel, handleEscape, handleRemoveFromEditForm]);

  /**
   * Render collapsed summary
   */
  const renderCollapsedSummary = useCallback(() => {
    const entries = Object.entries(labels);
    if (entries.length === 0) return null;

    return (
      <div className="flex flex-wrap gap-2">
        {entries.map(([key, value]) => (
          <span key={key} className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded-full">
            {key}: {value}
          </span>
        ))}
      </div>
    );
  }, [labels]);

  return (
    <CollapsibleFormSection
      ref={ref}
      sectionId="labels"
      title="Labels"
      icon={<Tag className="w-5 h-5" />}
      colorScheme="blue"
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
      navigationCallbacks={navigationCallbacks}
      items={chipItems}
      onAddItem={handleAddItem}
      renderEditForm={renderEditForm}
      renderCollapsedSummary={renderCollapsedSummary}
      isEditing={editingLabelKey !== null}
      editingIndex={editingLabelKey === null ? -1 : Object.keys(labels).indexOf(editingLabelKey)}
      onEditChip={handleEditChip}
      onRemoveChip={handleRemoveChip}
    />
  );
});

LabelsSection.displayName = 'LabelsSection';
