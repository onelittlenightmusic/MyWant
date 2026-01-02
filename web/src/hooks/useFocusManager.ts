import { useState, useCallback, useRef } from 'react';
import { FocusField, FocusManager } from '@/types/formSection';

/**
 * useFocusManager - Declarative focus management hook
 *
 * Replaces manual handleArrowKeyNavigation functions with a centralized
 * focus management system that tracks registered fields and provides
 * navigation callbacks.
 *
 * Usage:
 * ```tsx
 * const focusManager = useFocusManager();
 *
 * // Register fields (typically in useEffect)
 * useEffect(() => {
 *   focusManager.registerField({
 *     id: 'type-selector',
 *     order: 0,
 *     ref: typeSelectorRef
 *   });
 *   return () => focusManager.unregisterField('type-selector');
 * }, []);
 *
 * // Navigate between fields
 * <CollapsibleFormSection
 *   navigationCallbacks={{
 *     onNavigateUp: focusManager.navigateToPrev,
 *     onNavigateDown: focusManager.navigateToNext
 *   }}
 * />
 * ```
 */
export const useFocusManager = (): FocusManager => {
  // Use ref to avoid re-renders when fields are registered/unregistered
  const fieldsRef = useRef<Map<string, FocusField>>(new Map());
  const [, setVersion] = useState(0); // For forcing re-renders if needed

  /**
   * Register a field for focus management
   */
  const registerField = useCallback((field: FocusField) => {
    fieldsRef.current.set(field.id, field);
    setVersion(v => v + 1); // Trigger re-render if needed
  }, []);

  /**
   * Unregister a field
   */
  const unregisterField = useCallback((id: string) => {
    fieldsRef.current.delete(id);
    setVersion(v => v + 1); // Trigger re-render if needed
  }, []);

  /**
   * Get sorted fields by order
   */
  const getSortedFields = useCallback((): FocusField[] => {
    return Array.from(fieldsRef.current.values()).sort((a, b) => a.order - b.order);
  }, []);

  /**
   * Get current focused field index
   */
  const getCurrentFieldIndex = useCallback((): number => {
    const sortedFields = getSortedFields();
    const activeElement = document.activeElement;

    return sortedFields.findIndex(field => {
      const element = field.ref.current;
      // Check if the active element is the field itself or a descendant
      return element === activeElement || element?.contains(activeElement);
    });
  }, [getSortedFields]);

  /**
   * Navigate to the next field in sequence
   */
  const navigateToNext = useCallback(() => {
    const sortedFields = getSortedFields();
    if (sortedFields.length === 0) return;

    const currentIndex = getCurrentFieldIndex();

    // If no field is focused, focus the first one
    if (currentIndex === -1) {
      sortedFields[0]?.ref.current?.focus();
      return;
    }

    // Focus next field (wrap to first if at end)
    const nextIndex = (currentIndex + 1) % sortedFields.length;
    sortedFields[nextIndex]?.ref.current?.focus();
  }, [getSortedFields, getCurrentFieldIndex]);

  /**
   * Navigate to the previous field in sequence
   */
  const navigateToPrev = useCallback(() => {
    const sortedFields = getSortedFields();
    if (sortedFields.length === 0) return;

    const currentIndex = getCurrentFieldIndex();

    // If no field is focused, focus the last one
    if (currentIndex === -1) {
      sortedFields[sortedFields.length - 1]?.ref.current?.focus();
      return;
    }

    // Focus previous field (wrap to last if at beginning)
    const prevIndex = currentIndex === 0 ? sortedFields.length - 1 : currentIndex - 1;
    sortedFields[prevIndex]?.ref.current?.focus();
  }, [getSortedFields, getCurrentFieldIndex]);

  /**
   * Focus a specific field by ID
   */
  const focusField = useCallback((id: string) => {
    const field = fieldsRef.current.get(id);
    if (field?.ref.current) {
      field.ref.current.focus();
    }
  }, []);

  return {
    registerField,
    unregisterField,
    navigateToNext,
    navigateToPrev,
    focusField,
  };
};
