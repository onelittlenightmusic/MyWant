import React, { useMemo, useCallback, useRef, useState, useEffect } from 'react';
import { Want } from '@/types/want';
import { WantTypeListItem } from '@/types/wantType';
import { getStatusHexColor } from './WantCard/parts/StatusColor';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useWantStore } from '@/stores/wantStore';
import { classNames } from '@/utils/helpers';
import { WantCardFace } from './WantCardFace';
import { getPatternColor } from './WantTypeVisuals';
import { CanvasChildOverlay } from './CanvasChildOverlay';

const CELL_SIZE = 110;
const GAP = 6;
const STEP = CELL_SIZE + GAP;
/** Half-size of the default virtual grid. (0,0) is the center; cells range [-HALF, +HALF]. */
const CANVAS_HALF = 15;
const MIN_SCALE = 0.3;
const MAX_SCALE = 2.5;
const SCALE_STEP = 0.1;

export const CANVAS_LABEL_X = 'mywant.io/canvas-x';
export const CANVAS_LABEL_Y = 'mywant.io/canvas-y';

const isActiveStatus = (status: string) =>
  status === 'reaching' || status === 'initializing' || status === 'reaching_with_warning';

// Pre-compute spiral coordinates emanating from (0,0) outward.
// Uses a larger range than CANVAS_HALF so auto-placement works even with many wants.
function buildSpiralCoords(maxRings: number): ReadonlyArray<{ x: number; y: number }> {
  const result: Array<{ x: number; y: number }> = [{ x: 0, y: 0 }];
  for (let k = 1; k <= maxRings; k++) {
    for (let x = -k; x <= k; x++) result.push({ x, y: -k });      // top edge
    for (let y = -k + 1; y <= k; y++) result.push({ x: k, y });   // right edge
    for (let x = k - 1; x >= -k; x--) result.push({ x, y: k });  // bottom edge
    for (let y = k - 1; y >= -k + 1; y--) result.push({ x: -k, y }); // left edge
  }
  return result;
}
const SPIRAL_COORDS = buildSpiralCoords(50);

interface WantCanvasProps {
  wants: Want[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onCreateWant: (canvasX: number, canvasY: number) => void;
  onMoveWant: (wantId: string, x: number, y: number) => void;
  scale?: number;
  onScaleChange?: (scale: number) => void;
  /** Pre-rendered card to show centered over the selected tile */
  floatCard?: React.ReactNode;
  /** Children of the selected want, grouped into the role overlay */
  childWants?: Want[];
  onDeselect?: () => void;
  /** Extra controls rendered in the top-left toolbar (e.g. list/canvas toggle) */
  toolbarContent?: React.ReactNode;
}

export const WantCanvas: React.FC<WantCanvasProps> = ({
  wants,
  selectedWant,
  onViewWant,
  onCreateWant,
  onMoveWant,
  scale: scaleProp = 1.0,
  onScaleChange,
  floatCard,
  childWants = [],
  onDeselect: _onDeselect,
  toolbarContent,
}) => {
  const canvasRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [dragWantId, setDragWantId] = useState<string | null>(null);
  const [dragOverCell, setDragOverCell] = useState<{ x: number; y: number } | null>(null);
  const scale = scaleProp;
  const [tileCenter, setTileCenter] = useState<{ x: number; y: number } | null>(null);

  // Ref so non-passive event handlers can read latest scale without re-registering
  const scaleRef = useRef(scale);
  useEffect(() => { scaleRef.current = scale; }, [scale]);

  const lastPinchDist = useRef<number | null>(null);

  const wantTypes = useWantTypeStore(state => state.wantTypes);
  const typeMap = useMemo(() => {
    const m = new Map<string, WantTypeListItem>();
    wantTypes.forEach(wt => m.set(wt.name, wt));
    return m;
  }, [wantTypes]);

  const clampScale = (v: number) => Math.max(MIN_SCALE, Math.min(MAX_SCALE, v));

  // Zoom keeping the viewport center fixed in canvas-coordinate space.
  const applyScaleWithCenter = useCallback((newScale: number) => {
    const clamped = clampScale(newScale);
    const el = scrollRef.current;
    if (!el) { onScaleChange?.(clamped); return; }
    const cur = scaleRef.current;
    const vw = el.clientWidth;
    const vh = el.clientHeight;
    const cx = (el.scrollLeft + vw / 2) / cur;
    const cy = (el.scrollTop  + vh / 2) / cur;
    onScaleChange?.(clamped);
    requestAnimationFrame(() => {
      if (!scrollRef.current) return;
      scrollRef.current.scrollLeft = Math.max(0, cx * clamped - vw / 2);
      scrollRef.current.scrollTop  = Math.max(0, cy * clamped - vh / 2);
    });
  }, [onScaleChange]);

  const zoomIn  = () => applyScaleWithCenter(Math.round((scale + SCALE_STEP) * 10) / 10);
  const zoomOut = () => applyScaleWithCenter(Math.round((scale - SCALE_STEP) * 10) / 10);

  // Non-passive wheel + pinch listeners (React's synthetic handlers are passive)
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        // Zoom: keep viewport center fixed
        e.preventDefault();
        const cur = scaleRef.current;
        const factor = e.deltaY < 0 ? 1.1 : 0.9;
        const next = clampScale(Math.round(cur * factor * 100) / 100);
        const vw = el.clientWidth;
        const vh = el.clientHeight;
        const cx = (el.scrollLeft + vw / 2) / cur;
        const cy = (el.scrollTop  + vh / 2) / cur;
        onScaleChange?.(next);
        requestAnimationFrame(() => {
          if (!scrollRef.current) return;
          scrollRef.current.scrollLeft = Math.max(0, cx * next - vw / 2);
          scrollRef.current.scrollTop  = Math.max(0, cy * next - vh / 2);
        });
      } else if (e.shiftKey) {
        // Shift+Wheel → horizontal scroll (for mice without horizontal wheel)
        e.preventDefault();
        el.scrollLeft += e.deltaY;
      }
      // else: native vertical / trackpad-horizontal scroll
    };

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length !== 2) return;
      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      lastPinchDist.current = Math.sqrt(dx * dx + dy * dy);
    };

    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 2) return;
      e.preventDefault();
      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (lastPinchDist.current !== null) {
        const cur = scaleRef.current;
        const next = clampScale(Math.round(cur * (dist / lastPinchDist.current) * 100) / 100);
        const vw = el.clientWidth;
        const vh = el.clientHeight;
        const cx = (el.scrollLeft + vw / 2) / cur;
        const cy = (el.scrollTop  + vh / 2) / cur;
        onScaleChange?.(next);
        requestAnimationFrame(() => {
          if (!scrollRef.current) return;
          scrollRef.current.scrollLeft = Math.max(0, cx * next - vw / 2);
          scrollRef.current.scrollTop  = Math.max(0, cy * next - vh / 2);
        });
      }
      lastPinchDist.current = dist;
    };

    const onTouchEnd = () => { lastPinchDist.current = null; };

    el.addEventListener('wheel', onWheel, { passive: false });
    el.addEventListener('touchstart', onTouchStart, { passive: false });
    el.addEventListener('touchmove', onTouchMove, { passive: false });
    el.addEventListener('touchend', onTouchEnd);
    return () => {
      el.removeEventListener('wheel', onWheel);
      el.removeEventListener('touchstart', onTouchStart);
      el.removeEventListener('touchmove', onTouchMove);
      el.removeEventListener('touchend', onTouchEnd);
    };
  }, [onScaleChange]); // no `scale` dependency — reads via scaleRef

  // Optimistic local overrides
  const [localOverrides, setLocalOverrides] = useState<Map<string, { x: number; y: number }>>(new Map());

  useEffect(() => {
    setLocalOverrides(prev => {
      if (prev.size === 0) return prev;
      const next = new Map(prev);
      wants.forEach(want => {
        const id = want.metadata?.id || want.id;
        if (!id) return;
        const ov = next.get(id);
        if (!ov) return;
        const sx = parseInt(want.metadata?.labels?.[CANVAS_LABEL_X] ?? '', 10);
        const sy = parseInt(want.metadata?.labels?.[CANVAS_LABEL_Y] ?? '', 10);
        if (sx === ov.x && sy === ov.y) next.delete(id);
      });
      return next.size !== prev.size ? next : prev;
    });
  }, [wants]);

  // Combined layout: positionMap + canvas bounds + origin offset for (0,0).
  // originX/Y translate grid coords → pixel coords: pixel = (gridCoord + origin) * STEP
  const { positionMap, cols, rows, originX, originY } = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>();
    const occupied = new Set<string>();

    // First pass: saved / overridden positions
    wants.forEach(want => {
      const id = want.metadata?.id || want.id;
      if (!id) return;
      const ov = localOverrides.get(id);
      if (ov) {
        const key = `${ov.x},${ov.y}`;
        if (!occupied.has(key)) { map.set(id, ov); occupied.add(key); }
        return;
      }
      const rawX = want.metadata?.labels?.[CANVAS_LABEL_X];
      const rawY = want.metadata?.labels?.[CANVAS_LABEL_Y];
      if (rawX !== undefined && rawY !== undefined) {
        const x = parseInt(rawX, 10);
        const y = parseInt(rawY, 10);
        if (!isNaN(x) && !isNaN(y)) {
          const key = `${x},${y}`;
          if (!occupied.has(key)) { map.set(id, { x, y }); occupied.add(key); }
        }
      }
    });

    // Second pass: spiral auto-assign from center outward
    let si = 0;
    wants.forEach(want => {
      const id = want.metadata?.id || want.id;
      if (!id || map.has(id)) return;
      while (si < SPIRAL_COORDS.length) {
        const coord = SPIRAL_COORDS[si++];
        const key = `${coord.x},${coord.y}`;
        if (!occupied.has(key)) { map.set(id, { ...coord }); occupied.add(key); break; }
      }
    });

    // Canvas bounds: at least CANVAS_HALF in every direction with 2-cell padding
    let minX = -CANVAS_HALF, maxX = CANVAS_HALF;
    let minY = -CANVAS_HALF, maxY = CANVAS_HALF;
    map.forEach(({ x, y }) => {
      if (x - 2 < minX) minX = x - 2;
      if (x + 2 > maxX) maxX = x + 2;
      if (y - 2 < minY) minY = y - 2;
      if (y + 2 > maxY) maxY = y + 2;
    });

    return {
      positionMap: map,
      cols: maxX - minX + 1,
      rows: maxY - minY + 1,
      originX: -minX,  // pixel offset: cell (0,0) → pixel (originX * STEP + GAP/2)
      originY: -minY,
    };
  }, [wants, localOverrides]);

  // Scroll once on mount so (0,0) appears at the viewport center.
  // Double RAF ensures the browser has computed layout before reading clientWidth/Height.
  useEffect(() => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const el = scrollRef.current;
        if (!el) return;
        const s = scaleRef.current;
        el.scrollLeft = Math.max(0, CANVAS_HALF * STEP * s - el.clientWidth  / 2);
        el.scrollTop  = Math.max(0, CANVAS_HALF * STEP * s - el.clientHeight / 2);
      });
    });
  }, []); // mount only

  // Track viewport center of the selected tile for the overlay
  const updateTileCenter = useCallback(() => {
    if (!floatCard || !selectedWant || !scrollRef.current) { setTileCenter(null); return; }
    const id = selectedWant.metadata?.id || selectedWant.id;
    const pos = id ? positionMap.get(id) : undefined;
    if (!pos) { setTileCenter(null); return; }

    const el = scrollRef.current;
    const rect = el.getBoundingClientRect();
    const tileLeft = (pos.x + originX) * STEP + GAP / 2;
    const tileTop  = (pos.y + originY) * STEP + GAP / 2;
    setTileCenter({
      x: rect.left + (tileLeft + CELL_SIZE / 2) * scale - el.scrollLeft,
      y: rect.top  + (tileTop  + CELL_SIZE / 2) * scale - el.scrollTop,
    });
  }, [floatCard, selectedWant, positionMap, scale, originX, originY]);

  useEffect(() => { updateTileCenter(); }, [updateTileCenter]);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.addEventListener('scroll', updateTileCenter);
    window.addEventListener('resize', updateTileCenter);
    return () => {
      el.removeEventListener('scroll', updateTileCenter);
      window.removeEventListener('resize', updateTileCenter);
    };
  }, [updateTileCenter]);

  // Convert a mouse/drag event position to grid coordinates (possibly negative)
  const cellFromEvent = useCallback((e: React.MouseEvent | React.DragEvent) => {
    const rect = canvasRef.current!.getBoundingClientRect();
    return {
      cx: Math.floor((e.clientX - rect.left) / (STEP * scale)) - originX,
      cy: Math.floor((e.clientY - rect.top)  / (STEP * scale)) - originY,
    };
  }, [scale, originX, originY]);

  const handleCanvasClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (dragWantId) return;
    const { cx, cy } = cellFromEvent(e);
    const occupied = Array.from(positionMap.values()).some(p => p.x === cx && p.y === cy);
    if (!occupied) onCreateWant(cx, cy);
  }, [dragWantId, positionMap, onCreateWant, cellFromEvent]);

  const isCellOccupied = useCallback((cx: number, cy: number, excludeId?: string) => {
    for (const [id, pos] of positionMap.entries()) {
      if (id === excludeId) continue;
      if (pos.x === cx && pos.y === cy) return true;
    }
    return false;
  }, [positionMap]);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!e.dataTransfer.types.includes('application/mywant-canvas-id')) return;
    e.preventDefault();
    const { cx, cy } = cellFromEvent(e);
    setDragOverCell({ x: cx, y: cy });
  }, [cellFromEvent]);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    const wantId = e.dataTransfer.getData('application/mywant-canvas-id');
    if (!wantId) return;
    const { cx, cy } = cellFromEvent(e);
    if (!isCellOccupied(cx, cy, wantId)) {
      setLocalOverrides(prev => new Map(prev).set(wantId, { x: cx, y: cy }));
      onMoveWant(wantId, cx, cy);
    }
    setDragWantId(null);
    setDragOverCell(null);
  }, [onMoveWant, isCellOccupied, cellFromEvent]);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect();
    if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) {
      setDragOverCell(null);
    }
  }, []);

  const canvasW = cols * STEP + GAP;
  const canvasH = rows * STEP + GAP;

  return (
    <div className="w-full flex-1 relative" style={{ minHeight: 0 }}>
      {/* Toolbar: list/canvas toggle + zoom controls — fixed top-left of canvas */}
      <div className="absolute top-2 left-2 z-50 flex items-center gap-2 pointer-events-none select-none">
        {toolbarContent && (
          <div className="pointer-events-auto flex items-center">
            {toolbarContent}
          </div>
        )}
        <div className="flex items-center gap-1">
          <button
            className="pointer-events-auto w-7 h-7 rounded bg-white/10 hover:bg-white/20 text-white text-lg font-bold flex items-center justify-center transition-colors"
            onClick={zoomOut} title="Zoom out"
          >−</button>
          <span
            className="pointer-events-auto text-white/70 text-xs font-mono w-12 text-center cursor-pointer hover:text-white transition-colors"
            onClick={() => applyScaleWithCenter(1.0)} title="Reset zoom"
          >{Math.round(scale * 100)}%</span>
          <button
            className="pointer-events-auto w-7 h-7 rounded bg-white/10 hover:bg-white/20 text-white text-lg font-bold flex items-center justify-center transition-colors"
            onClick={zoomIn} title="Zoom in"
          >+</button>
        </div>
      </div>

      {/* Scroll container */}
      <div ref={scrollRef} className="overflow-auto w-full h-full">
        {/* Spacer drives scrollbars at scaled size */}
        <div style={{ width: canvasW * scale, height: canvasH * scale, position: 'relative' }}>
          <div
            ref={canvasRef}
            className="relative select-none"
            style={{
              width: canvasW,
              height: canvasH,
              backgroundColor: '#0a0f1e',
              backgroundImage: [
                'linear-gradient(rgba(148,163,184,0.07) 1px, transparent 1px)',
                'linear-gradient(90deg, rgba(148,163,184,0.07) 1px, transparent 1px)',
              ].join(', '),
              backgroundSize: `${STEP}px ${STEP}px`,
              backgroundPosition: `${GAP / 2}px ${GAP / 2}px`,
              cursor: dragWantId ? 'grabbing' : 'crosshair',
              transform: `scale(${scale})`,
              transformOrigin: '0 0',
              position: 'absolute',
              top: 0, left: 0,
            }}
            onClick={handleCanvasClick}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            onDragLeave={handleDragLeave}
          >
            {/* Drag-over highlight */}
            {dragOverCell && (() => {
              const blocked = isCellOccupied(dragOverCell.x, dragOverCell.y, dragWantId ?? undefined);
              return (
                <div
                  className="absolute pointer-events-none rounded z-0"
                  style={{
                    left: (dragOverCell.x + originX) * STEP + GAP / 2,
                    top:  (dragOverCell.y + originY) * STEP + GAP / 2,
                    width: CELL_SIZE, height: CELL_SIZE,
                    border: blocked ? '2px solid rgba(239,68,68,0.7)' : '2px solid rgba(96,165,250,0.6)',
                    backgroundColor: blocked ? 'rgba(239,68,68,0.12)' : 'rgba(96,165,250,0.08)',
                  }}
                />
              );
            })()}

            {/* Want tiles */}
            {wants.map(want => {
              const id = want.metadata?.id || want.id;
              if (!id) return null;
              const pos = positionMap.get(id);
              if (!pos) return null;

              const isSelected = (selectedWant?.metadata?.id || selectedWant?.id) === id;
              const isDragging = dragWantId === id;
              const statusColor = getStatusHexColor(want.status);
              const type = want.metadata?.type || '';
              const wantTypeInfo = typeMap.get(type);
              const category = wantTypeInfo?.category ?? '';
              const pattern = wantTypeInfo?.pattern ?? '';
              const name = want.metadata?.name || id;
              const active = isActiveStatus(want.status);
              const patColor = getPatternColor(pattern);

              return (
                <WantCardFace
                  key={id}
                  typeName={type}
                  displayName={name}
                  category={category}
                  theme="dark"
                  iconSize={26}
                  className={classNames(
                    'rounded z-10 transition-transform duration-100',
                    !isDragging && 'hover:scale-[1.06] hover:z-20',
                    isSelected && 'scale-[1.06] z-30',
                    isDragging && 'opacity-40',
                  )}
                  style={{
                    position: 'absolute',
                    left: (pos.x + originX) * STEP + GAP / 2,
                    top:  (pos.y + originY) * STEP + GAP / 2,
                    width: CELL_SIZE, height: CELL_SIZE,
                    boxShadow: isSelected
                      ? `0 0 0 2px #fff, 0 0 0 4px ${statusColor}, 0 6px 20px rgba(0,0,0,0.7)`
                      : active
                      ? `0 0 14px ${patColor}60, 0 2px 10px rgba(0,0,0,0.6)`
                      : '0 2px 10px rgba(0,0,0,0.6)',
                    cursor: isDragging ? 'grabbing' : 'grab',
                  }}
                  dataAttributes={{ 'data-want-id': id }}
                  onClick={e => { e.stopPropagation(); onViewWant(want); }}
                  onContextMenu={e => {
                    e.preventDefault(); e.stopPropagation();
                    onViewWant(want);
                    useWantStore.getState().setQuickActionsWantId(id);
                  }}
                  draggable
                  onDragStart={e => {
                    e.dataTransfer.setData('application/mywant-canvas-id', id);
                    e.dataTransfer.effectAllowed = 'move';
                    setDragWantId(id);
                  }}
                  onDragEnd={() => { setDragWantId(null); setDragOverCell(null); }}
                >
                  <div
                    className={classNames('absolute top-0 left-0 right-0 z-20', active && 'animate-pulse')}
                    style={{ height: 3, backgroundColor: statusColor }}
                  />
                  {active && (
                    <div
                      className="absolute top-2 right-2 z-20 rounded-full animate-pulse"
                      style={{ width: 5, height: 5, backgroundColor: statusColor }}
                    />
                  )}
                  {isSelected && (
                    <div
                      className="absolute inset-0 pointer-events-none rounded z-30"
                      style={{ border: `2px solid ${statusColor}`, opacity: 0.85 }}
                    />
                  )}
                </WantCardFace>
              );
            })}
          </div>
        </div>
      </div>

      {/* Child overlay: float card centered on tile + child mini-tiles by role */}
      {floatCard && tileCenter && (
        <CanvasChildOverlay
          floatCard={floatCard}
          childWants={childWants}
          tileCenterX={tileCenter.x}
          tileCenterY={tileCenter.y}
          onClickChild={onViewWant}
        />
      )}
    </div>
  );
};
