/**
 * Shared label utility functions used across the application
 * Provides a unified approach for adding labels through the API
 */

/**
 * Add a label to the global label registry via the backend API
 * This allows labels to be registered even if they don't exist on any want yet
 *
 * @param key - The label key
 * @param value - The label value
 * @returns Promise<boolean> - true if successful, false otherwise
 */
export async function addLabelToRegistry(key: string, value: string): Promise<boolean> {
  try {
    const trimmedKey = key.trim();
    const trimmedValue = value.trim();

    if (!trimmedKey || !trimmedValue) {
      console.error('Label key and value must not be empty');
      return false;
    }

    const response = await fetch('http://localhost:8080/api/v1/labels', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        key: trimmedKey,
        value: trimmedValue
      })
    });

    if (!response.ok) {
      console.error('Failed to add label:', response.statusText);
      return false;
    }

    return true;
  } catch (error) {
    console.error('Error adding label:', error);
    return false;
  }
}

/**
 * Validate label input
 * @param key - The label key
 * @param value - The label value
 * @returns string[] - Array of error messages, empty if valid
 */
export function validateLabel(key: string, value: string): string[] {
  const errors: string[] = [];

  const trimmedKey = key.trim();
  const trimmedValue = value.trim();

  if (!trimmedKey) {
    errors.push('Label key is required');
  }

  if (!trimmedValue) {
    errors.push('Label value is required');
  }

  if (trimmedKey.length > 100) {
    errors.push('Label key must be less than 100 characters');
  }

  if (trimmedValue.length > 100) {
    errors.push('Label value must be less than 100 characters');
  }

  return errors;
}
