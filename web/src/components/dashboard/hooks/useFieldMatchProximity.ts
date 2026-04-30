import { useState, useRef, useCallback } from 'react';

export type ProximityDirection = 'left' | 'right' | 'above' | 'below';

export interface FieldMatchRec {
  score: number;
  description: string;
  source: { want_id: string; want_name: string; field_name: string; field_type: string; label: string; is_final: boolean };
  target: { want_id: string; want_name: string; param_name: string };
  param_change: { want_id: string; param_name: string; value: unknown };
}

export interface ProximityState {
  /** The want whose state fields are exposed (the data provider). */
  sourceId: string;
  /** The want whose params receive the imported field (the data consumer). */
  targetId: string;
  /** Spatial direction: where dropped is relative to near. */
  direction: ProximityDirection;
  /** Which labels were exposed for this proximity check (e.g. ["current"] or ["plan","goal"]). */
  exposedLabels: string[];
  recs: FieldMatchRec[];
  /** Canvas-space midpoint between the two cards (unscaled) */
  midX: number;
  midY: number;
}

interface Options {
  positionMap: Map<string, { x: number; y: number }>;
  step: number;
  cellSize: number;
  originX: number;
  originY: number;
}

const PROXIMITY_GRID_DIST = 1.5;

/**
 * Classify the spatial direction of `dropped` relative to `near`.
 * The dominant axis (|dx| vs |dy|) wins; ties favour horizontal.
 */
function classifyDirection(
  dropped: { x: number; y: number },
  near: { x: number; y: number },
): ProximityDirection {
  const dx = dropped.x - near.x;
  const dy = dropped.y - near.y;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? 'right' : 'left';
  }
  return dy >= 0 ? 'below' : 'above';
}

/**
 * Resolve the GPC role of (source, target, exposedLabels) from the spatial direction.
 *
 * Spatial model:
 *   horizontal (left/right) → exposes `current`
 *   vertical   (above/below) → exposes `plan` and `goal`
 *
 * Provider/consumer assignment:
 *   left  → dropped is provider (left-of-near = exposes towards near)
 *   right → near    is provider (right-of-near = receives from near)
 *   above → dropped is provider (above-of-near = exposes plan/goal down)
 *   below → near    is provider (below-of-near = receives plan/goal from above)
 */
function rolesForDirection(
  direction: ProximityDirection,
  draggedId: string,
  nearId: string,
): { sourceId: string; targetId: string; exposedLabels: string[] } {
  const horizontal = direction === 'left' || direction === 'right';
  const exposedLabels = horizontal ? ['current'] : ['plan', 'goal'];

  // Dropped is the "provider" when placed left or above.
  const droppedIsProvider = direction === 'left' || direction === 'above';
  return {
    sourceId: droppedIsProvider ? draggedId : nearId,
    targetId: droppedIsProvider ? nearId : draggedId,
    exposedLabels,
  };
}

export function useFieldMatchProximity({ positionMap, step, cellSize, originX, originY }: Options) {
  const [proximity, setProximity] = useState<ProximityState | null>(null);
  const dismissedRef = useRef<Set<string>>(new Set());

  /**
   * Call on drop. Finds the nearest card to droppedCell, classifies the spatial direction,
   * and fetches recommendations with the appropriate exposed_labels.
   */
  const checkOnDrop = useCallback(async (dragId: string, droppedCell: { x: number; y: number }) => {
    let nearId: string | null = null;
    let nearDist = Infinity;
    let nearPos: { x: number; y: number } | null = null;
    for (const [id, pos] of positionMap.entries()) {
      if (id === dragId) continue;
      const dx = pos.x - droppedCell.x;
      const dy = pos.y - droppedCell.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (dist <= PROXIMITY_GRID_DIST && dist < nearDist) {
        nearDist = dist;
        nearId = id;
        nearPos = pos;
      }
    }

    if (!nearId || !nearPos) return;

    const direction = classifyDirection(droppedCell, nearPos);
    const { sourceId, targetId, exposedLabels } = rolesForDirection(direction, dragId, nearId);

    // Dismissal key includes direction so flipping between axes still re-shows recs.
    const pairKey = `${sourceId}::${targetId}::${direction}`;
    if (dismissedRef.current.has(pairKey)) return;

    try {
      const labelsParam = exposedLabels.join(',');
      const res = await fetch(
        `/api/v1/wants/field-match-recommendations?source_id=${sourceId}&target_id=${targetId}&exposed_labels=${labelsParam}`,
      );
      if (!res.ok) return;
      const data = await res.json();
      const recs: FieldMatchRec[] = data.recommendations ?? [];
      if (recs.length === 0) return;

      const aPos = droppedCell;
      const bPos = nearPos;
      const ax = (aPos.x + originX) * step + cellSize / 2;
      const ay = (aPos.y + originY) * step + cellSize / 2;
      const bx = (bPos.x + originX) * step + cellSize / 2;
      const by = (bPos.y + originY) * step + cellSize / 2;

      setProximity({
        sourceId,
        targetId,
        direction,
        exposedLabels,
        recs,
        midX: (ax + bx) / 2,
        midY: (ay + by) / 2,
      });
    } catch {
      // network error — silently ignore
    }
  }, [positionMap, originX, originY, step, cellSize]);

  /** Dismiss this pair+direction — won't re-show until next drag session. */
  const dismiss = useCallback((sourceId: string, targetId: string, direction: ProximityDirection) => {
    dismissedRef.current.add(`${sourceId}::${targetId}::${direction}`);
    setProximity(null);
  }, []);

  const clear = useCallback(() => {
    setProximity(null);
  }, []);

  /** Reset dismissed pairs at the start of each drag so the user can retry. */
  const resetDismissed = useCallback(() => {
    dismissedRef.current = new Set();
  }, []);

  return { proximity, checkOnDrop, clear, dismiss, resetDismissed };
}
