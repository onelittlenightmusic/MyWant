import React, { useState, useCallback, useMemo, forwardRef, useRef } from 'react';
import { GitBranch } from 'lucide-react';
import { CollapsibleFormSection } from '../CollapsibleFormSection';
import { LabelSelectorAutocomplete } from '../LabelSelectorAutocomplete';
import { ChipItem, SectionNavigationCallbacks } from '@/types/formSection';
import { CommitInputHandle } from '@/components/common/CommitInput';

/**
 * Props for DependenciesSection
 */
interface DependenciesSectionProps {
  /** Current dependencies as array of single-entry objects */
  dependencies: Array<Record<string, string>>;
  /** Callback when dependencies change */
  onChange: (dependencies: Array<Record<string, string>>) => void;
  /** Whether the section is collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
}

/**
 * Editing state for a dependency
 */
interface DependencyDraft {
  key: string;
  value: string;
}

/**
 * DependenciesSection - Adapter for dependencies using CollapsibleFormSection
 *
 * Manages dependencies as Array<Record<string, string>> internally where each
 * record has exactly one key-value pair representing a label selector.
 */
export const DependenciesSection = forwardRef<HTMLButtonElement, DependenciesSectionProps>(({
  dependencies,
  onChange,
  isCollapsed,
  onToggleCollapse,
  navigationCallbacks,
}, ref) => {
  // Editing state
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [editingDraft, setEditingDraft] = useState<DependencyDraft>({ key: '', value: '' });

  // Refs for focus management and force commit
  const headerRef = useRef<HTMLButtonElement>(null);
  const autocompleteRef = useRef<CommitInputHandle>(null);

  // Merge forwarded ref with local headerRef
  React.useEffect(() => {
    if (typeof ref === 'function') {
      ref(headerRef.current);
    } else if (ref) {
      (ref as React.MutableRefObject<HTMLButtonElement | null>).current = headerRef.current;
    }
  }, [ref]);

  // Auto-focus first input when editing starts
  React.useEffect(() => {
    if (editingIndex !== null && autocompleteRef.current) {
      setTimeout(() => {
        autocompleteRef.current?.focus();
      }, 100);
    }
  }, [editingIndex]);

  /**
   * Convert dependencies to chip items for display
   */
  const chipItems = useMemo((): ChipItem[] => {
    return dependencies
      .map((dep, index) => {
        // Skip if this dependency is being edited
        if (index === editingIndex) return null;

        // Get the single key-value pair from the dependency object
        const [key, value] = Object.entries(dep)[0];
        return {
          key: `${index}`,
          display: `${key}: ${value}`,
        };
      })
      .filter((item): item is ChipItem => item !== null);
  }, [dependencies, editingIndex]);

  /**
   * Start editing an existing dependency
   */
  const handleEditChip = useCallback((chipIndex: number) => {
    // Map chip index to dependency index (accounting for hidden editing chip)
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }

    const dependency = dependencies[actualIndex];
    if (dependency) {
      const [key, value] = Object.entries(dependency)[0];
      setEditingIndex(actualIndex);
      setEditingDraft({ key, value });
    }
  }, [dependencies, editingIndex]);

  /**
   * Remove a dependency
   */
  const handleRemoveChip = useCallback((chipIndex: number) => {
    // Map chip index to dependency index
    let actualIndex = chipIndex;
    if (editingIndex !== null && chipIndex >= editingIndex) {
      actualIndex = chipIndex + 1;
    }

    const newDependencies = dependencies.filter((_, index) => index !== actualIndex);
    onChange(newDependencies);
  }, [dependencies, editingIndex, onChange]);

  /**
   * Start adding a new dependency
   */
  const handleAddItem = useCallback(() => {
    setEditingIndex(dependencies.length);
    setEditingDraft({ key: '', value: '' });
  }, [dependencies.length]);

  /**
   * Save the current dependency edit
   */
  const handleSave = useCallback(() => {
    // Force commit uncommitted values from the autocomplete inputs
    autocompleteRef.current?.commit();

    // Only update if draft has a key value
    if (editingDraft.key.trim()) {
      const newDependencies = [...dependencies];

      // Create new dependency object with single key-value pair
      const newDependency = { [editingDraft.key]: editingDraft.value };

      if (editingIndex !== null && editingIndex < dependencies.length) {
        // Replace existing dependency
        newDependencies[editingIndex] = newDependency;
      } else {
        // Add new dependency
        newDependencies.push(newDependency);
      }

      onChange(newDependencies);
    }

    // Reset editing state
    setEditingIndex(null);
    setEditingDraft({ key: '', value: '' });
  }, [editingIndex, editingDraft, dependencies, onChange]);

  /**
   * Cancel the current dependency edit
   */
  const handleCancel = useCallback(() => {
    setEditingIndex(null);
    setEditingDraft({ key: '', value: '' });
  }, []);

  /**
   * Handle escape key - cancel and return focus to header
   */
  const handleEscape = useCallback(() => {
    handleCancel();
    setTimeout(() => {
      headerRef.current?.focus();
    }, 0);
  }, [handleCancel]);

  /**
   * Handle remove from edit form
   */
  const handleRemoveFromEditForm = useCallback(() => {
    if (editingIndex !== null && editingIndex < dependencies.length) {
      const newDependencies = dependencies.filter((_, index) => index !== editingIndex);
      onChange(newDependencies);
    }

    setEditingIndex(null);
    setEditingDraft({ key: '', value: '' });
  }, [editingIndex, dependencies, onChange]);

  /**
   * Render the edit form
   */
  const renderEditForm = useCallback(() => (
    <div className="space-y-3">
      <LabelSelectorAutocomplete
        ref={autocompleteRef}
        keyValue={editingDraft.key}
        valuValue={editingDraft.value}
        onKeyChange={(newKey) => setEditingDraft(prev => ({ ...prev, key: newKey }))}
        onValueChange={(newValue) => setEditingDraft(prev => ({ ...prev, value: newValue }))}
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
  ), [editingDraft, handleSave, handleCancel, handleEscape, handleRemoveFromEditForm]);

  /**
   * Render collapsed summary
   */
  const renderCollapsedSummary = useCallback(() => {
    if (dependencies.length === 0) return null;

    return (
      <div className="flex flex-wrap gap-2">
        {dependencies.map((dep, index) => {
          const [key, value] = Object.entries(dep)[0];
          return (
            <span key={index} className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded-full">
              {key}: {value}
            </span>
          );
        })}
      </div>
    );
  }, [dependencies]);

  return (
    <CollapsibleFormSection
      ref={headerRef}
      sectionId="dependencies"
      title="Using"
      icon={<GitBranch className="w-5 h-5" />}
      colorScheme="green"
      isCollapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
      navigationCallbacks={navigationCallbacks}
      items={chipItems}
      onAddItem={handleAddItem}
      renderEditForm={renderEditForm}
      renderCollapsedSummary={renderCollapsedSummary}
      isEditing={editingIndex !== null}
      editingIndex={editingIndex ?? -1}
      onEditChip={handleEditChip}
      onRemoveChip={handleRemoveChip}
    />
  );
});

DependenciesSection.displayName = 'DependenciesSection';
