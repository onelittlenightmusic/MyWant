import { useState, useRef, useCallback } from 'react';

export interface FieldMatchRec {
  score: number;
  description: string;
  source: { want_id: string; want_name: string; field_name: string; field_type: string; is_final: boolean };
  target: { want_id: string; want_name: string; param_name: string };
  param_change: { want_id: string; param_name: string; value: unknown };
}

export interface ProximityState {
  sourceId: string;
  targetId: string;
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

export function useFieldMatchProximity({ positionMap, step, cellSize, originX, originY }: Options) {
  const [proximity, setProximity] = useState<ProximityState | null>(null);
  const dismissedRef = useRef<Set<string>>(new Set());

  /**
   * Call on drop. Finds the nearest card to droppedCell and immediately fetches recommendations.
   * No debounce — the drop itself is the trigger.
   */
  const checkOnDrop = useCallback(async (dragId: string, droppedCell: { x: number; y: number }) => {
    let nearId: string | null = null;
    let nearDist = Infinity;
    for (const [id, pos] of positionMap.entries()) {
      if (id === dragId) continue;
      const dx = pos.x - droppedCell.x;
      const dy = pos.y - droppedCell.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (dist <= PROXIMITY_GRID_DIST && dist < nearDist) {
        nearDist = dist;
        nearId = id;
      }
    }

    if (!nearId) return;

    const pairKey = `${dragId}::${nearId}`;
    if (dismissedRef.current.has(pairKey)) return;

    try {
      const res = await fetch(`/api/v1/wants/field-match-recommendations?source_id=${dragId}&target_id=${nearId}`);
      if (!res.ok) return;
      const data = await res.json();
      const recs: FieldMatchRec[] = data.recommendations ?? [];
      if (recs.length === 0) return;

      const aPos = droppedCell;
      const bPos = positionMap.get(nearId)!;
      const ax = (aPos.x + originX) * step + cellSize / 2;
      const ay = (aPos.y + originY) * step + cellSize / 2;
      const bx = (bPos.x + originX) * step + cellSize / 2;
      const by = (bPos.y + originY) * step + cellSize / 2;

      setProximity({ sourceId: dragId, targetId: nearId, recs, midX: (ax + bx) / 2, midY: (ay + by) / 2 });
    } catch {
      // network error — silently ignore
    }
  }, [positionMap, originX, originY, step, cellSize]);

  /** Dismiss this pair — won't re-show until next drag session. */
  const dismiss = useCallback((sourceId: string, targetId: string) => {
    dismissedRef.current.add(`${sourceId}::${targetId}`);
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
