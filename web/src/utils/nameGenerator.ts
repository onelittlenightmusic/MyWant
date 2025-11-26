/**
 * Utility functions for generating want names automatically
 */

/**
 * Generate a want name based on the selected type/recipe
 * Format: "{typeName}-{suffix}" or "{typeName}" if no suffix
 *
 * @param selectedId - The ID of the selected want type or recipe
 * @param itemType - Whether it's a 'want-type' or 'recipe'
 * @param userInput - Optional user input to append as suffix
 * @returns Generated want name
 */
export function generateWantName(
  selectedId: string,
  itemType: 'want-type' | 'recipe',
  userInput?: string
): string {
  if (!selectedId) {
    return '';
  }

  // Convert type name to kebab-case (handle spaces and underscores)
  const baseName = selectedId
    .toLowerCase()
    .replace(/\s+/g, '-')
    .replace(/_/g, '-')
    .replace(/--+/g, '-');

  // If user provided input, append it as suffix
  if (userInput && userInput.trim()) {
    const suffix = userInput
      .trim()
      .toLowerCase()
      .replace(/\s+/g, '-')
      .replace(/_/g, '-')
      .replace(/--+/g, '-');

    return `${baseName}-${suffix}`;
  }

  // Default suffix based on type
  const defaultSuffix = itemType === 'recipe' ? 'example' : 'instance';
  return `${baseName}-${defaultSuffix}`;
}

/**
 * Suggest variations of a want name
 * Useful for when a name already exists
 *
 * @param baseName - The base name to suggest variations for
 * @param existingNames - Set of already used names
 * @returns A name that's not in the existing names set
 */
export function suggestAlternativeName(
  baseName: string,
  existingNames: Set<string>
): string {
  if (!existingNames.has(baseName)) {
    return baseName;
  }

  // Try appending numbers
  for (let i = 1; i <= 100; i++) {
    const candidate = `${baseName}-${i}`;
    if (!existingNames.has(candidate)) {
      return candidate;
    }
  }

  // Fallback: use timestamp
  return `${baseName}-${Date.now()}`;
}

/**
 * Generate a unique want name that doesn't conflict with existing wants
 * Tries different descriptive suffixes before falling back to numbers
 *
 * @param selectedId - The ID of the selected want type or recipe
 * @param itemType - Whether it's a 'want-type' or 'recipe'
 * @param existingNames - Set of already used names
 * @param userInput - Optional user input to append as suffix
 * @returns A unique want name
 */
export function generateUniqueWantName(
  selectedId: string,
  itemType: 'want-type' | 'recipe',
  existingNames: Set<string>,
  userInput?: string
): string {
  if (!selectedId) {
    return '';
  }

  // If user provided custom input, use suggestAlternativeName on that
  if (userInput && userInput.trim()) {
    const customName = generateWantName(selectedId, itemType, userInput);
    return suggestAlternativeName(customName, existingNames);
  }

  // Convert type name to kebab-case
  const baseName = selectedId
    .toLowerCase()
    .replace(/\s+/g, '-')
    .replace(/_/g, '-')
    .replace(/--+/g, '-');

  // Try default suffix first
  const defaultSuffix = itemType === 'recipe' ? 'example' : 'instance';
  const defaultName = `${baseName}-${defaultSuffix}`;

  if (!existingNames.has(defaultName)) {
    return defaultName;
  }

  // Try alternative descriptive suffixes
  const alternatives = itemType === 'recipe'
    ? ['example', 'demo', 'template', 'sample']
    : ['instance', 'copy', 'test', 'new'];

  for (const suffix of alternatives) {
    const candidate = `${baseName}-${suffix}`;
    if (!existingNames.has(candidate)) {
      return candidate;
    }
  }

  // Fallback to numbered suffix
  return suggestAlternativeName(defaultName, existingNames);
}

/**
 * Validate a want name
 * @param name - The name to validate
 * @returns True if valid, false otherwise
 */
export function isValidWantName(name: string): boolean {
  if (!name || !name.trim()) {
    return false;
  }

  // Allow alphanumeric, hyphens, underscores
  // Must start with alphanumeric or underscore
  const nameRegex = /^[a-zA-Z0-9_][a-zA-Z0-9_-]*$/;
  return nameRegex.test(name.trim());
}

/**
 * Sanitize a want name to be valid
 * @param name - The name to sanitize
 * @returns Sanitized name
 */
export function sanitizeWantName(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]/g, '-')
    .replace(/^-+/, '') // Remove leading hyphens
    .replace(/-+$/, '') // Remove trailing hyphens
    .replace(/-+/g, '-') // Collapse multiple hyphens
    || 'want'; // Fallback
}
