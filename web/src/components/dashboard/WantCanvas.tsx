import React, { useMemo, useCallback, useRef, useState, useEffect } from 'react';
import { Want } from '@/types/want';
import { getStatusHexColor } from './WantCard/parts/StatusColor';
import { classNames } from '@/utils/helpers';

const CELL_SIZE = 110;
const GAP = 6;
const STEP = CELL_SIZE + GAP;
const MIN_COLS = 30;
const MIN_ROWS = 30;
const MIN_SCALE = 0.3;
const MAX_SCALE = 2.5;
const SCALE_STEP = 0.1;

export const CANVAS_LABEL_X = 'mywant.io/canvas-x';
export const CANVAS_LABEL_Y = 'mywant.io/canvas-y';

const TYPE_GRADIENT: Record<string, string> = {
  whim: 'linear-gradient(160deg, #4c1d95 0%, #1e1b4b 100%)',
  'whim-target': 'linear-gradient(160deg, #4c1d95 0%, #1e1b4b 100%)',
  goal: 'linear-gradient(160deg, #1e3a5f 0%, #0f172a 100%)',
  travel: 'linear-gradient(160deg, #064e3b 0%, #0f172a 100%)',
  itinerary: 'linear-gradient(160deg, #064e3b 0%, #0f172a 100%)',
  reminder: 'linear-gradient(160deg, #78350f 0%, #1c1917 100%)',
  command: 'linear-gradient(160deg, #7c2d12 0%, #1c1917 100%)',
  draft: 'linear-gradient(160deg, #1e293b 0%, #0f172a 100%)',
};
const DEFAULT_GRADIENT = 'linear-gradient(160deg, #1e293b 0%, #0f172a 100%)';

const TYPE_EMOJI: Record<string, string> = {
  whim: '✨',
  'whim-target': '✨',
  goal: '🎯',
  travel: '✈️',
  itinerary: '🗺️',
  reminder: '⏰',
  command: '💻',
  flight: '✈️',
  hotel: '🏨',
  draft: '📝',
};
const DEFAULT_EMOJI = '💎';

const getGradient = (type: string) => {
  const t = type.toLowerCase();
  for (const k of Object.keys(TYPE_GRADIENT)) {
    if (t.includes(k)) return TYPE_GRADIENT[k];
  }
  return DEFAULT_GRADIENT;
};

const getEmoji = (type: string) => {
  const t = type.toLowerCase();
  for (const k of Object.keys(TYPE_EMOJI)) {
    if (t.includes(k)) return TYPE_EMOJI[k];
  }
  return DEFAULT_EMOJI;
};

const isActiveStatus = (status: string) =>
  status === 'reaching' || status === 'initializing' || status === 'reaching_with_warning';

interface WantCanvasProps {
  wants: Want[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onCreateWant: (canvasX: number, canvasY: number) => void;
  onMoveWant: (wantId: string, x: number, y: number) => void;
  scale?: number;
  onScaleChange?: (scale: number) => void;
}

export const WantCanvas: React.FC<WantCanvasProps> = ({
  wants,
  selectedWant,
  onViewWant,
  onCreateWant,
  onMoveWant,
  scale: scaleProp = 1.0,
  onScaleChange,
}) => {
  const canvasRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [dragWantId, setDragWantId] = useState<string | null>(null);
  const [dragOverCell, setDragOverCell] = useState<{ x: number; y: number } | null>(null);
  const scale = scaleProp;
  const lastPinchDist = useRef<number | null>(null);

  const clampScale = (v: number) => Math.max(MIN_SCALE, Math.min(MAX_SCALE, v));
  const applyScale = useCallback((v: number) => { onScaleChange?.(clampScale(v)); }, [onScaleChange]);
  const zoomIn = () => applyScale(Math.round((scale + SCALE_STEP) * 10) / 10);
  const zoomOut = () => applyScale(Math.round((scale - SCALE_STEP) * 10) / 10);

  // Non-passive wheel + touch listeners so preventDefault works (React handlers are passive)
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
        const factor = e.deltaY < 0 ? 1.1 : 0.9;
        onScaleChange?.(clampScale(Math.round(scale * factor * 100) / 100));
      }
    };

    const onTouchStart = (e: TouchEvent) => {
      if (e.touches.length === 2) {
        const dx = e.touches[0].clientX - e.touches[1].clientX;
        const dy = e.touches[0].clientY - e.touches[1].clientY;
        lastPinchDist.current = Math.sqrt(dx * dx + dy * dy);
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 2) return;
      e.preventDefault();
      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      const dist = Math.sqrt(dx * dx + dy * dy);
      if (lastPinchDist.current !== null) {
        const factor = dist / lastPinchDist.current;
        onScaleChange?.(clampScale(Math.round(scale * factor * 100) / 100));
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
  }, [scale, onScaleChange]);

  // Optimistic local overrides: applied immediately on drop, cleared when backend confirms
  // Map<wantId, {x, y}>
  const [localOverrides, setLocalOverrides] = useState<Map<string, { x: number; y: number }>>(
    new Map()
  );

  // When `wants` changes (after fetchWants), clear overrides whose new label matches
  useEffect(() => {
    setLocalOverrides(prev => {
      if (prev.size === 0) return prev;
      const next = new Map(prev);
      wants.forEach(want => {
        const id = want.metadata?.id || want.id;
        if (!id) return;
        const override = next.get(id);
        if (!override) return;
        const savedX = parseInt(want.metadata?.labels?.[CANVAS_LABEL_X] ?? '', 10);
        const savedY = parseInt(want.metadata?.labels?.[CANVAS_LABEL_Y] ?? '', 10);
        // Backend confirmed the position → remove override
        if (savedX === override.x && savedY === override.y) next.delete(id);
      });
      return next.size !== prev.size ? next : prev;
    });
  }, [wants]);

  // Compute display positions for all wants (backend labels + local overrides)
  const positionMap = useMemo(() => {
    const map = new Map<string, { x: number; y: number }>();
    const occupied = new Set<string>();

    // First pass: wants with saved canvas positions (or local overrides)
    wants.forEach(want => {
      const id = want.metadata?.id || want.id;
      if (!id) return;

      // Local override takes precedence (optimistic update)
      const override = localOverrides.get(id);
      if (override) {
        map.set(id, override);
        occupied.add(`${override.x},${override.y}`);
        return;
      }

      const rawX = want.metadata?.labels?.[CANVAS_LABEL_X];
      const rawY = want.metadata?.labels?.[CANVAS_LABEL_Y];
      if (rawX !== undefined && rawY !== undefined) {
        const x = parseInt(rawX, 10);
        const y = parseInt(rawY, 10);
        if (!isNaN(x) && !isNaN(y) && x >= 0 && y >= 0) {
          map.set(id, { x, y });
          occupied.add(`${x},${y}`);
        }
      }
    });

    // Second pass: auto-assign for wants without saved position
    let nx = 0;
    let ny = 0;
    const advance = () => {
      nx++;
      if (nx >= MIN_COLS) { nx = 0; ny++; }
    };

    wants.forEach(want => {
      const id = want.metadata?.id || want.id;
      if (!id || map.has(id)) return;
      while (occupied.has(`${nx},${ny}`)) advance();
      map.set(id, { x: nx, y: ny });
      occupied.add(`${nx},${ny}`);
      advance();
    });

    return map;
  }, [wants, localOverrides]);

  // Canvas size: enough to fit all wants + some empty space
  const { cols, rows } = useMemo(() => {
    let maxX = MIN_COLS - 1;
    let maxY = MIN_ROWS - 1;
    positionMap.forEach(({ x, y }) => {
      if (x > maxX) maxX = x;
      if (y > maxY) maxY = y;
    });
    return { cols: maxX + 3, rows: maxY + 3 };
  }, [positionMap]);

  const cellFromEvent = (e: React.MouseEvent | React.DragEvent) => {
    const rect = canvasRef.current!.getBoundingClientRect();
    const cx = Math.floor((e.clientX - rect.left) / (STEP * scale));
    const cy = Math.floor((e.clientY - rect.top) / (STEP * scale));
    return { cx, cy };
  };

  const handleCanvasClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (dragWantId) return;
    const { cx, cy } = cellFromEvent(e);
    if (cx < 0 || cy < 0 || cx >= cols || cy >= rows) return;
    const occupied = Array.from(positionMap.values()).some(p => p.x === cx && p.y === cy);
    if (!occupied) {
      onCreateWant(cx, cy);
    }
  }, [dragWantId, positionMap, cols, rows, onCreateWant]);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    if (!e.dataTransfer.types.includes('application/mywant-canvas-id')) return;
    e.preventDefault();
    const { cx, cy } = cellFromEvent(e);
    setDragOverCell({ x: cx, y: cy });
  }, []);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    const wantId = e.dataTransfer.getData('application/mywant-canvas-id');
    if (!wantId) return;
    const { cx, cy } = cellFromEvent(e);
    if (cx >= 0 && cy >= 0) {
      // Apply optimistic local update immediately so the block doesn't snap back
      setLocalOverrides(prev => new Map(prev).set(wantId, { x: cx, y: cy }));
      onMoveWant(wantId, cx, cy);
    }
    setDragWantId(null);
    setDragOverCell(null);
  }, [onMoveWant]);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    // Only clear when actually leaving the canvas (not a child element)
    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect();
    if (
      e.clientX < rect.left || e.clientX > rect.right ||
      e.clientY < rect.top || e.clientY > rect.bottom
    ) {
      setDragOverCell(null);
    }
  }, []);

  return (
    <div className="w-full flex-1 relative" style={{ minHeight: 0 }}>
      {/* Zoom controls */}
      <div className="absolute top-2 left-2 z-50 flex items-center gap-1 pointer-events-none select-none">
        <button
          className="pointer-events-auto w-7 h-7 rounded bg-white/10 hover:bg-white/20 text-white text-lg font-bold flex items-center justify-center transition-colors"
          onClick={zoomOut}
          title="Zoom out"
        >−</button>
        <span
          className="pointer-events-auto text-white/70 text-xs font-mono w-12 text-center cursor-pointer hover:text-white transition-colors"
          onClick={() => applyScale(1.0)}
          title="Reset zoom"
        >{Math.round(scale * 100)}%</span>
        <button
          className="pointer-events-auto w-7 h-7 rounded bg-white/10 hover:bg-white/20 text-white text-lg font-bold flex items-center justify-center transition-colors"
          onClick={zoomIn}
          title="Zoom in"
        >+</button>
      </div>

      {/* Scroll container */}
      <div
        ref={scrollRef}
        className="overflow-auto w-full h-full"
      >
        {/* Spacer div to drive scrollbars at the scaled size */}
        <div style={{ width: (cols * STEP + GAP) * scale, height: (rows * STEP + GAP) * scale, position: 'relative' }}>
          <div
            ref={canvasRef}
            className="relative select-none"
            style={{
              width: cols * STEP + GAP,
              height: rows * STEP + GAP,
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
              top: 0,
              left: 0,
            }}
            onClick={handleCanvasClick}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            onDragLeave={handleDragLeave}
          >
        {/* Drag-over highlight cell */}
        {dragOverCell && (
          <div
            className="absolute pointer-events-none rounded z-0"
            style={{
              left: dragOverCell.x * STEP + GAP / 2,
              top: dragOverCell.y * STEP + GAP / 2,
              width: CELL_SIZE,
              height: CELL_SIZE,
              border: '2px solid rgba(96,165,250,0.6)',
              backgroundColor: 'rgba(96,165,250,0.08)',
            }}
          />
        )}

        {/* Want blocks */}
        {wants.map(want => {
          const id = want.metadata?.id || want.id;
          if (!id) return null;
          const pos = positionMap.get(id);
          if (!pos) return null;

          const isSelected = (selectedWant?.metadata?.id || selectedWant?.id) === id;
          const isDragging = dragWantId === id;
          const statusColor = getStatusHexColor(want.status);
          const type = want.metadata?.type || '';
          const gradient = getGradient(type);
          const emoji = getEmoji(type);
          const name = want.metadata?.name || id;
          const active = isActiveStatus(want.status);

          return (
            <div
              key={id}
              data-want-id={id}
              className={classNames(
                'absolute rounded overflow-hidden z-10',
                'transition-transform duration-100',
                !isDragging && 'hover:scale-[1.06] hover:z-20',
                isSelected && 'scale-[1.06] z-30',
                isDragging && 'opacity-40',
              )}
              style={{
                left: pos.x * STEP + GAP / 2,
                top: pos.y * STEP + GAP / 2,
                width: CELL_SIZE,
                height: CELL_SIZE,
                background: gradient,
                boxShadow: isSelected
                  ? `0 0 0 2px #fff, 0 0 0 4px ${statusColor}, 0 6px 20px rgba(0,0,0,0.6)`
                  : active
                  ? `0 0 12px ${statusColor}50, 0 2px 8px rgba(0,0,0,0.5)`
                  : '0 2px 8px rgba(0,0,0,0.5)',
                cursor: isDragging ? 'grabbing' : 'grab',
              }}
              onClick={e => { e.stopPropagation(); onViewWant(want); }}
              draggable
              onDragStart={e => {
                e.dataTransfer.setData('application/mywant-canvas-id', id);
                e.dataTransfer.effectAllowed = 'move';
                setDragWantId(id);
              }}
              onDragEnd={() => { setDragWantId(null); setDragOverCell(null); }}
            >
              {/* Status bar (top) */}
              <div
                className={classNames('absolute top-0 left-0 right-0', active && 'animate-pulse')}
                style={{ height: 3, backgroundColor: statusColor }}
              />

              {/* Content */}
              <div className="flex flex-col items-center justify-center h-full gap-1 px-2 pt-1">
                <span
                  className="leading-none"
                  style={{ fontSize: 28, filter: 'drop-shadow(0 1px 2px rgba(0,0,0,0.5))' }}
                >
                  {emoji}
                </span>

                <div
                  className={classNames('rounded-full', active && 'animate-pulse')}
                  style={{ width: 6, height: 6, backgroundColor: statusColor, flexShrink: 0 }}
                />

                <span
                  className="text-white/75 font-medium text-center leading-tight"
                  style={{
                    fontSize: 10,
                    overflow: 'hidden',
                    display: '-webkit-box',
                    WebkitLineClamp: 2,
                    WebkitBoxOrient: 'vertical' as const,
                    wordBreak: 'break-all',
                    maxWidth: '100%',
                  }}
                >
                  {name}
                </span>
              </div>

              {/* Selected overlay */}
              {isSelected && (
                <div
                  className="absolute inset-0 pointer-events-none rounded"
                  style={{ border: `2px solid ${statusColor}`, opacity: 0.5 }}
                />
              )}
            </div>
          );
        })}

        {/* Empty cell hint on hover (CSS-only via cursor:crosshair) */}
          </div>
        </div>
      </div>
    </div>
  );
};
