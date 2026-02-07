/**
 * Fractional Indexing Utility
 *
 * Generates lexicographically sortable strings that allow infinite insertions
 * between any two positions. Ideal for drag-and-drop reordering.
 *
 * Based on: https://www.figma.com/blog/realtime-editing-of-ordered-sequences/
 */

const BASE_CHARS = '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz';
const BASE = BASE_CHARS.length; // 62

/**
 * Generate the first order key
 */
export function generateFirstKey(): string {
  return 'a0';
}

/**
 * Generate a key after the given key
 * @param key - The key to generate after
 */
export function generateKeyAfter(key: string | null | undefined): string {
  if (!key) {
    return generateFirstKey();
  }

  // Increment the last character
  const lastChar = key[key.length - 1];
  const lastCharIndex = BASE_CHARS.indexOf(lastChar);

  if (lastCharIndex < BASE - 1) {
    // Can increment the last character
    return key.slice(0, -1) + BASE_CHARS[lastCharIndex + 1];
  } else {
    // Last character is at max, append new character
    return key + '0';
  }
}

/**
 * Generate a key before the given key
 * @param key - The key to generate before
 */
export function generateKeyBefore(key: string | null | undefined): string {
  if (!key) {
    return generateFirstKey();
  }

  // Decrement the last character
  const lastChar = key[key.length - 1];
  const lastCharIndex = BASE_CHARS.indexOf(lastChar);

  if (lastCharIndex > 0) {
    // Can decrement the last character
    return key.slice(0, -1) + BASE_CHARS[lastCharIndex - 1];
  } else {
    // Need to go to previous "digit"
    if (key.length === 1) {
      // Can't go before single character
      throw new Error('Cannot generate key before minimum key');
    }
    const prefix = key.slice(0, -1);
    return generateKeyBefore(prefix);
  }
}

/**
 * Generate a key between two keys
 * @param keyA - The first key (or null for start)
 * @param keyB - The second key (or null for end)
 */
export function generateKeyBetween(
  keyA: string | null | undefined,
  keyB: string | null | undefined
): string {
  // If no keys, return first key
  if (!keyA && !keyB) {
    return generateFirstKey();
  }

  // If only keyB exists, generate before it
  if (!keyA) {
    return generateKeyBefore(keyB);
  }

  // If only keyA exists, generate after it
  if (!keyB) {
    return generateKeyAfter(keyA);
  }

  // Both keys exist, generate between them
  // Find the first position where they differ
  const minLength = Math.min(keyA.length, keyB.length);
  let i = 0;

  while (i < minLength && keyA[i] === keyB[i]) {
    i++;
  }

  // They are identical up to the shorter length
  if (i === minLength) {
    // If keyA is shorter, we can append to it
    if (keyA.length < keyB.length) {
      const nextChar = keyB[i];
      const nextCharIndex = BASE_CHARS.indexOf(nextChar);

      if (nextCharIndex > 0) {
        // Can insert between by using a character before nextChar
        const midCharIndex = Math.floor(nextCharIndex / 2);
        return keyA + BASE_CHARS[midCharIndex];
      } else {
        // nextChar is '0', need to append to keyA
        return keyA + BASE_CHARS[Math.floor(BASE / 2)];
      }
    } else {
      // keyA is longer or equal, append to keyA
      return generateKeyAfter(keyA);
    }
  }

  // They differ at position i
  const charA = keyA[i];
  const charB = keyB[i];
  const indexA = BASE_CHARS.indexOf(charA);
  const indexB = BASE_CHARS.indexOf(charB);

  if (indexB - indexA > 1) {
    // There's room between the characters
    const midIndex = Math.floor((indexA + indexB) / 2);
    return keyA.slice(0, i) + BASE_CHARS[midIndex];
  } else {
    // Characters are adjacent, need to go deeper
    // Use the first key as prefix and append a middle character
    const midCharIndex = Math.floor(BASE / 2);
    return keyA.slice(0, i + 1) + BASE_CHARS[midCharIndex];
  }
}

/**
 * Generate multiple sequential keys for initial creation
 * @param count - Number of keys to generate
 * @param startKey - Optional starting key
 */
export function generateSequentialKeys(count: number, startKey?: string): string[] {
  const keys: string[] = [];
  let currentKey = startKey || generateFirstKey();

  for (let i = 0; i < count; i++) {
    keys.push(currentKey);
    currentKey = generateKeyAfter(currentKey);
  }

  return keys;
}

/**
 * Sort items by their order keys
 */
export function sortByOrderKey<T>(items: T[], getOrderKey: (item: T) => string | undefined): T[] {
  return [...items].sort((a, b) => {
    const keyA = getOrderKey(a) || '';
    const keyB = getOrderKey(b) || '';
    return keyA.localeCompare(keyB);
  });
}

/**
 * Migrate existing items to use fractional indexing
 * Assigns sequential keys based on current order
 */
export function migrateToFractionalIndexing<T>(
  items: T[],
  getCurrentOrder: (item: T) => number | string
): Map<T, string> {
  // Sort by current order
  const sorted = [...items].sort((a, b) => {
    const orderA = getCurrentOrder(a);
    const orderB = getCurrentOrder(b);

    if (typeof orderA === 'number' && typeof orderB === 'number') {
      return orderA - orderB;
    }
    return String(orderA).localeCompare(String(orderB));
  });

  // Generate sequential keys
  const keys = generateSequentialKeys(sorted.length);
  const mapping = new Map<T, string>();

  sorted.forEach((item, index) => {
    mapping.set(item, keys[index]);
  });

  return mapping;
}
