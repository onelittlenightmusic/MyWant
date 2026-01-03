/**
 * Type definitions for generic form section components
 * Used across CollapsibleFormSection, FocusableChip, and section adapters
 */

/**
 * Section identifiers for the WantForm
 */
export type SectionId = 'parameters' | 'labels' | 'dependencies' | 'scheduling';

/**
 * Color schemes for chips and sections
 * - blue: Labels, Dependencies, Parameters
 * - amber: Scheduling (with clock icon)
 * - green: (reserved for future use)
 */
export type ColorScheme = 'blue' | 'amber' | 'green';

/**
 * Data structure for chip items displayed in sections
 */
export interface ChipItem {
  /** Unique identifier for the chip */
  key: string;
  /** Display text shown on the chip */
  display: string;
  /** Optional icon component to display */
  icon?: React.ReactNode;
}

/**
 * Callbacks for navigation between sections
 * Used by CollapsibleFormSection to communicate with parent form
 */
export interface SectionNavigationCallbacks {
  /** Navigate to the previous section */
  onNavigateUp: () => void;
  /** Navigate to the next section */
  onNavigateDown: () => void;
  /** Navigate via Tab key (e.g. to action button) */
  onTab?: () => void;
}

/**
 * Callbacks for edit form operations
 * Used by section adapters to handle save/cancel/escape actions
 */
export interface EditFormCallbacks {
  /** Save the current edit and exit edit mode */
  onSave: () => void;
  /** Cancel the current edit and discard changes */
  onCancel: () => void;
  /** Escape key pressed - cancel and return focus to header */
  onEscape: () => void;
}

/**
 * Callbacks for chip navigation
 * Used by FocusableChip to navigate between chips
 */
export interface ChipNavigationCallbacks {
  /** Navigate to the next chip (right arrow) */
  onNavigateNext?: () => void;
  /** Navigate to the previous chip (left arrow) */
  onNavigatePrev?: () => void;
  /** Return focus to section header (escape key) */
  onEscape?: () => void;
}

/**
 * Props for the FocusableChip component
 */
export interface FocusableChipProps extends ChipNavigationCallbacks {
  /** Chip data to display */
  item: ChipItem;
  /** Color scheme for the chip */
  colorScheme: ColorScheme;
  /** Whether this chip is currently focused */
  isFocused?: boolean;
  /** Callback when chip is clicked or Enter is pressed */
  onEdit: () => void;
  /** Callback when remove button is clicked */
  onRemove: () => void;
}

/**
 * Props for the CollapsibleFormSection component
 */
export interface CollapsibleFormSectionProps {
  /** Unique section identifier */
  sectionId: SectionId;
  /** Section title displayed in header */
  title: string;
  /** Icon component for the section header */
  icon: React.ReactNode;
  /** Color scheme for the section */
  colorScheme: ColorScheme;
  /** Whether the section is currently collapsed */
  isCollapsed: boolean;
  /** Callback to toggle collapsed/expanded state */
  onToggleCollapse: () => void;
  /** Navigation callbacks for moving between sections */
  navigationCallbacks: SectionNavigationCallbacks;
  /** Items to display as chips */
  items: ChipItem[];
  /** Callback when 'a' key is pressed on header (add new item) */
  onAddItem: () => void;
  /** Render function for the edit form */
  renderEditForm: () => React.ReactNode;
  /** Render function for the collapsed summary */
  renderCollapsedSummary: () => React.ReactNode;
  /** Whether currently in edit mode */
  isEditing: boolean;
  /** Index of the chip being edited (-1 if creating new) */
  editingIndex: number;
  /** Callback when a chip is clicked or Enter is pressed */
  onEditChip: (index: number) => void;
  /** Callback when a chip's remove button is clicked */
  onRemoveChip: (index: number) => void;
}

/**
 * Field registration for useFocusManager hook
 */
export interface FocusField {
  /** Field identifier */
  id: string;
  /** Order in the focus sequence (lower = earlier) */
  order: number;
  /** Ref to the focusable element */
  ref: React.RefObject<HTMLElement>;
}

/**
 * Return type for useFocusManager hook
 */
export interface FocusManager {
  /** Register a field for focus management */
  registerField: (field: FocusField) => void;
  /** Unregister a field */
  unregisterField: (id: string) => void;
  /** Navigate to the next field in sequence */
  navigateToNext: () => void;
  /** Navigate to the previous field in sequence */
  navigateToPrev: () => void;
  /** Focus a specific field by ID */
  focusField: (id: string) => void;
}
