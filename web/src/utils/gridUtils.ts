/**
 * Column width reference: 80% of the want detail sidebar width (480px → 384px).
 * A new column is added whenever the container has room for another 384px column.
 */
export const GRID_COLUMN_WIDTH = 384;

/**
 * Computes the number of grid columns that fit in the given container width.
 */
export function computeGridColumns(containerWidth: number): number {
  return Math.max(1, Math.floor(containerWidth / GRID_COLUMN_WIDTH));
}
