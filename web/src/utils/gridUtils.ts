/**
 * Column width reference: matches the want detail sidebar width (480px).
 * A new column is added whenever the container has room for another 480px column.
 */
export const GRID_COLUMN_WIDTH = 480;

/**
 * Computes the number of grid columns that fit in the given container width.
 */
export function computeGridColumns(containerWidth: number): number {
  return Math.max(1, Math.floor(containerWidth / GRID_COLUMN_WIDTH));
}
