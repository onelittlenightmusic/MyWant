import { useState, useRef, useCallback, useEffect } from 'react';
import { Want } from '@/types/want';

export interface FieldMatchRec {
  id: string;
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
  wants: Want[];
  step: number;
  cellSize: number;
  originX: number;
  originY: number;
}

const PROXIMITY_GRID_DIST = 1.5; // grid units — adjacent counts as near
const DEBOUNCE_MS = 400;

export function useFieldMatchProximity({ positionMap, wants, step, cellSize, originX, originY }: Options) {
  const [proximity, setProximity] = useState<ProximityState | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastPairRef = useRef<string>('');
  const dismissedRef = useRef<Set<string>>(new Set());

  const check = useCallback((dragId: string | null, overCell: { x: number; y: number } | null) => {
    if (!dragId || !overCell) return;

    // Find any non-dragged card that is within PROXIMITY_GRID_DIST of overCell
    let nearId: string | null = null;
    let nearDist = Infinity;
    for (const [id, pos] of positionMap.entries()) {
      if (id === dragId) continue;
      const dx = pos.x - overCell.x;
      const dy = pos.y - overCell.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (dist <= PROXIMITY_GRID_DIST && dist < nearDist) {
        nearDist = dist;
        nearId = id;
      }
    }

    if (!nearId) { setProximity(null); return; }

    const pairKey = `${dragId}::${nearId}`;
    if (dismissedRef.current.has(pairKey)) return;
    if (lastPairRef.current === pairKey) return; // already fetched

    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      try {
        const res = await fetch(`/api/v1/wants/field-match-recommendations?source_id=${dragId}&target_id=${nearId}`);
        if (!res.ok) return;
        const data = await res.json();
        const recs: FieldMatchRec[] = data.recommendations ?? [];
        if (recs.length === 0) return;

        lastPairRef.current = pairKey;

        // Midpoint in canvas-space (unscaled pixels)
        const aPos = positionMap.get(dragId) ?? overCell;
        const bPos = positionMap.get(nearId)!;
        const ax = (aPos.x + originX) * step + cellSize / 2;
        const ay = (aPos.y + originY) * step + cellSize / 2;
        const bx = (bPos.x + originX) * step + cellSize / 2;
        const by = (bPos.y + originY) * step + cellSize / 2;

        setProximity({ sourceId: dragId, targetId: nearId, recs, midX: (ax + bx) / 2, midY: (ay + by) / 2 });
      } catch {
        // network error — silently ignore
      }
    }, DEBOUNCE_MS);
  }, [positionMap, originX, originY, step, cellSize]);

  const dismiss = useCallback((sourceId: string, targetId: string) => {
    dismissedRef.current.add(`${sourceId}::${targetId}`);
    setProximity(null);
    lastPairRef.current = '';
  }, []);

  const clear = useCallback(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    setProximity(null);
    lastPairRef.current = '';
  }, []);

  // Reset dismissed set when wants change significantly (new drag session)
  useEffect(() => {
    dismissedRef.current = new Set();
  }, [wants.length]);

  useEffect(() => () => { if (debounceRef.current) clearTimeout(debounceRef.current); }, []);

  return { proximity, check, clear, dismiss };
}
