/**
 * Unified background style utilities for Want cards and details
 * Provides consistent background color and image selection logic across components
 */

export interface BackgroundStyle {
  className: string;
  style?: React.CSSProperties;
  hasBackgroundImage: boolean;
}

/**
 * Get background image URL based on want type
 * @param type - Want type (flight, hotel, restaurant, buffet, etc.)
 * @returns Image URL or undefined
 */
const GITHUB_RAW_BASE = 'https://raw.githubusercontent.com/onelittlenightmusic/MyWant/master/web/public/resources';

export const getBackgroundImage = (type?: string): string | undefined => {
  if (!type) return undefined;

  const imageMap: Record<string, string> = {
    flight: `${GITHUB_RAW_BASE}/flight.png`,
    hotel: `${GITHUB_RAW_BASE}/hotel.png`,
    restaurant: `${GITHUB_RAW_BASE}/restaurant.png`,
    buffet: `${GITHUB_RAW_BASE}/buffet.png`,
    evidence: `${GITHUB_RAW_BASE}/evidence.png`,
    // Mathematics category types and recipes
    'prime numbers': `${GITHUB_RAW_BASE}/numbers.png`,
    'prime sequence': `${GITHUB_RAW_BASE}/numbers.png`,
    'fibonacci numbers': `${GITHUB_RAW_BASE}/numbers.png`,
    'fibonacci filter': `${GITHUB_RAW_BASE}/numbers.png`,
    'fibonacci sequence': `${GITHUB_RAW_BASE}/numbers.png`,
    'prime sieve': `${GITHUB_RAW_BASE}/numbers.png`,
  };

  // Check exact type match first
  if (imageMap[type]) {
    return imageMap[type];
  }

  // Check if type ends with 'coordinator'
  if (type.endsWith('coordinator')) {
    return `${GITHUB_RAW_BASE}/agent.png`;
  }

  // System/Execution category - applies to scheduler, execution_result, execution result, and related types
  const systemTypes = [
    'scheduler',
    'execution_result',
    'execution result',
    'command execution',
    'command_execution'
  ];

  if (systemTypes.includes(type.toLowerCase()) || type.toLowerCase().includes('execution') || type.toLowerCase().includes('scheduler')) {
    return `${GITHUB_RAW_BASE}/screen.png`;
  }

  return undefined;
};

/**
 * Get background style for a want based on type and context
 * Returns consistent styling for both WantCard and WantDetailsSidebar
 *
 * @param type - Want type
 * @param isParentWant - Whether this is a parent want (affects background)
 * @returns BackgroundStyle object with className, style, and hasBackgroundImage flag
 */
export const getBackgroundStyle = (
  type?: string,
  isParentWant: boolean = false
): BackgroundStyle => {
  const backgroundImage = getBackgroundImage(type);
  const hasBackgroundImage = !!backgroundImage;

  // Determine if we should apply semi-transparent background
  // Parent wants always get semi-transparent background
  // Child wants get semi-transparent background only if they have a background image
  const shouldApplySemiTransparent = isParentWant || hasBackgroundImage;

  const className = shouldApplySemiTransparent
    ? 'bg-white bg-opacity-70 dark:bg-gray-800 dark:bg-opacity-80'
    : 'bg-white dark:bg-gray-800';

  const style: React.CSSProperties | undefined = backgroundImage
    ? {
        backgroundImage: `url(${backgroundImage})`,
        backgroundSize: 'cover',
        backgroundPosition: 'center center',
        backgroundRepeat: 'no-repeat',
        backgroundAttachment: 'scroll',
      }
    : undefined;

  return {
    className,
    style,
    hasBackgroundImage,
  };
};

/**
 * Get overlay style for parent want backgrounds
 * Creates the semi-transparent white overlay effect
 *
 * @returns CSS class for overlay
 */
export const getBackgroundOverlayClass = (): string => {
  return 'absolute inset-0 bg-white bg-opacity-70 dark:bg-gray-900 dark:bg-opacity-80 z-0 pointer-events-none';
};

/**
 * Get container style for content positioned over background image
 * Used when displaying content over a background image
 *
 * @returns CSS class for content container
 */
export const getBackgroundContentContainerClass = (hasBackgroundImage: boolean): string => {
  return hasBackgroundImage ? 'relative z-10' : '';
};
