import React, { forwardRef } from 'react';
import { X } from 'lucide-react';
import { FocusableChipProps, ColorScheme } from '@/types/formSection';

/**
 * FocusableChip - A chip component with keyboard navigation support
 *
 * Keyboard shortcuts:
 * - Enter: Start editing the chip
 * - Escape: Return focus to section header
 * - Arrow Left: Navigate to previous chip
 * - Arrow Right: Navigate to next chip
 * - Delete/Backspace: Remove the chip
 */
export const FocusableChip = forwardRef<HTMLButtonElement, FocusableChipProps>(({
  item,
  colorScheme,
  isFocused = false,
  onEdit,
  onRemove,
  onNavigateNext,
  onNavigatePrev,
  onEscape,
}, ref) => {

  /**
   * Get color classes based on color scheme
   */
  const getColorClasses = (scheme: ColorScheme): string => {
    switch (scheme) {
      case 'blue':
        return isFocused
          ? 'bg-blue-500 text-white border-blue-600 ring-2 ring-blue-300'
          : 'bg-blue-100 text-blue-800 border-blue-300 hover:bg-blue-200';
      case 'amber':
        return isFocused
          ? 'bg-amber-500 text-white border-amber-600 ring-2 ring-amber-300'
          : 'bg-amber-100 text-amber-800 border-amber-300 hover:bg-amber-200';
      case 'green':
        return isFocused
          ? 'bg-green-500 text-white border-green-600 ring-2 ring-green-300'
          : 'bg-green-100 text-green-800 border-green-300 hover:bg-green-200';
    }
  };

  /**
   * Get remove button color classes based on color scheme
   */
  const getRemoveButtonClasses = (scheme: ColorScheme): string => {
    if (isFocused) {
      return 'text-white hover:text-gray-200';
    }
    switch (scheme) {
      case 'blue':
        return 'text-blue-600 hover:text-blue-800';
      case 'amber':
        return 'text-amber-600 hover:text-amber-800';
      case 'green':
        return 'text-green-600 hover:text-green-800';
    }
  };

  /**
   * Handle keyboard navigation
   */
  const handleKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    // Arrow navigation
    if (e.key === 'ArrowRight' && onNavigateNext) {
      e.preventDefault();
      onNavigateNext();
    } else if (e.key === 'ArrowLeft' && onNavigatePrev) {
      e.preventDefault();
      onNavigatePrev();
    }
    // Enter to edit
    else if (e.key === 'Enter') {
      e.preventDefault();
      onEdit();
    }
    // Escape to return to header
    else if (e.key === 'Escape' && onEscape) {
      e.preventDefault();
      onEscape();
    }
    // Delete/Backspace to remove
    else if (e.key === 'Delete' || e.key === 'Backspace') {
      e.preventDefault();
      onRemove();
    }
  };

  /**
   * Handle remove button click
   */
  const handleRemoveClick = (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation(); // Prevent triggering chip click
    onRemove();
  };

  return (
    <button
      ref={ref}
      type="button"
      onClick={onEdit}
      onKeyDown={handleKeyDown}
      className={`
        inline-flex items-center gap-2 px-3 py-1.5 rounded-full
        border transition-all duration-150
        focus:outline-none font-medium text-sm
        ${getColorClasses(colorScheme)}
      `}
      aria-label={`${item.display} - Press Enter to edit, Delete to remove, Arrow keys to navigate`}
    >
      {/* Optional icon */}
      {item.icon && (
        <span className="flex-shrink-0">
          {item.icon}
        </span>
      )}

      {/* Chip text */}
      <span className="flex-shrink-0">
        {item.display}
      </span>

      {/* Remove button */}
      <button
        type="button"
        onClick={handleRemoveClick}
        className={`
          flex-shrink-0 ml-1 p-0.5 rounded-full
          transition-colors focus:outline-none
          ${getRemoveButtonClasses(colorScheme)}
        `}
        aria-label={`Remove ${item.display}`}
        tabIndex={-1} // Don't interfere with chip focus
      >
        <X className="w-3 h-3" />
      </button>
    </button>
  );
});

FocusableChip.displayName = 'FocusableChip';
