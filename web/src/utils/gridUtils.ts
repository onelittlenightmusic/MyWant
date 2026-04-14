/**
 * Column width reference: 80% of the want detail sidebar width (480px → 384px).
 * A new column is added whenever the container has room for another 384px column.
 */
export const GRID_COLUMN_WIDTH = 384;

/**
 * Computes the number of grid columns that fit in the given container width.
 * Accounts for responsive gaps used in the grid layout:
 * - gap-3 (12px) for small widths
 * - sm:gap-4 (16px) for width >= 640px
 * - lg:gap-6 (24px) for width >= 1024px
 * Uses window.innerWidth for gap breakpoints to match Tailwind media queries.
 */
export function computeGridColumns(containerWidth: number): number {
  // Determine gap size based on the window width (to match Tailwind media queries)
  const windowWidth = typeof window !== 'undefined' ? window.innerWidth : 1200;
  
  let gap = 12; // default gap-3
  if (windowWidth >= 1024) {
    gap = 24; // lg:gap-6
  } else if (windowWidth >= 640) {
    gap = 16; // sm:gap-4
  }

  // Formula for columns in a grid with gaps (auto-fill):
  // n * columnWidth + (n-1) * gap <= containerWidth
  // n * (columnWidth + gap) <= containerWidth + gap
  
  // Use a tiny epsilon (0.1px) for subpixel precision issues, 
  // but avoid over-optimism that causes row-end mismatch.
  const columns = Math.floor((containerWidth + gap + 0.1) / (GRID_COLUMN_WIDTH + gap));
  return Math.max(1, columns);
}
