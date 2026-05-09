import React, { useMemo, useCallback, useRef, useState, useEffect, useLayoutEffect, forwardRef, useImperativeHandle } from 'react';
import { Want } from '@/types/want';
import { WantTypeListItem } from '@/types/wantType';
import { getStatusHexColor } from './WantCard/parts/StatusColor';
import { useWantTypeStore } from '@/stores/wantTypeStore';
import { useWantStore } from '@/stores/wantStore';
import { useConfigStore } from '@/stores/configStore';
import { classNames } from '@/utils/helpers';
import { WantCardFace } from './WantCardFace';
import { getPatternColor } from './WantTypeVisuals';
import { useColorMode } from '@/hooks/useColorMode';
import { CanvasChildOverlay } from './CanvasChildOverlay';
import { useFieldMatchProximity, FieldMatchRec } from './hooks/useFieldMatchProximity';
import { FieldMatchBubble } from './FieldMatchBubble';

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

export interface WantCanvasRef {
  /** Enter keyboard-driven drag mode for the given wantId */
  startKeyboardDrag(wantId: string): void;
  /** Move the drag cursor one cell in the given direction */
  moveKeyboardDragCursor(dir: 'up' | 'down' | 'left' | 'right'): void;
  /** Confirm the current drag position — executes the move + triggers field match check */
  confirmKeyboardDrop(): void;
  /** Cancel keyboard drag and restore original position */
  cancelKeyboardDrag(): void;
  /** Returns true while a keyboard drag is active */
  isKeyboardDragging(): boolean;
}

interface WantCanvasProps {
  wants: Want[];
  selectedWant: Want | null;
  onViewWant: (want: Want) => void;
  onCreateWant: (canvasX: number, canvasY: number) => void;
  onMoveWant: (wantId: string, x: number, y: number) => void;
  scale?: number;
  onScaleChange?: (scale: number) => void;
  centerX?: number;
  centerY?: number;
  onCenterChange?: (x: number, y: number) => void;
  /** Pre-rendered card to show centered over the selected tile */
  floatCard?: React.ReactNode;
  /** Children of the selected want, grouped into the role overlay */
  childWants?: Want[];
  onDeselect?: () => void;
  /** Called when a want template (type/recipe) is dropped from outside */
  onTemplateDrop?: (templateId: string, itemType: 'want-type' | 'recipe', x: number, y: number) => void;
  /** Extra controls rendered in the top-left toolbar (e.g. list/canvas toggle) */
  toolbarContent?: React.ReactNode;
  /** wantId → correlation rate (0–N); populated when radar mode is active */
  correlationHighlights?: Map<string, number>;
}

export const WantCanvas = forwardRef<WantCanvasRef, WantCanvasProps>(({
  wants,
  selectedWant,
  onViewWant,
  onCreateWant,
  onMoveWant,
  scale: scaleProp = 1.0,
  onScaleChange,
  centerX,
  centerY,
  onCenterChange,
  floatCard,
  childWants = [],
  onDeselect: _onDeselect,
  onTemplateDrop,
  toolbarContent,
  correlationHighlights,
}, ref) => {
  const colorMode = useColorMode();
  const canvasRef = useRef<HTMLDivElement>(null);
  const spacerRef  = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const isPanningRef  = useRef(false);
  const panStartRef   = useRef<{ x: number; y: number; sl: number; st: number } | null>(null);
  const hasPannedRef  = useRef(false);
  const [isPanning, setIsPanning] = useState(false);
  const [dragWantId, setDragWantId] = useState<string | null>(null);
  const [dragOverCell, setDragOverCell] = useState<{ x: number; y: number } | null>(null);
  const kbDragActiveRef = useRef(false);
  const scale = scaleProp;
  const [tileCenter, setTileCenter] = useState<{ x: number; y: number } | null>(null);
  const [viewportSize, setViewportSize] = useState({ width: 0, height: 0 });

  // Tracks the last applied center to avoid feedback loops
  const lastAppliedCenterRef = useRef<{ x: number; y: number } | null>(null);
  const isUserScrollingRef = useRef(false);
  const scrollEndTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Internal function to calculate current viewport center in canvas-space
  const getCanvasCenter = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return null;
    const s = scaleRef.current;
    const vw = el.clientWidth;
    const vh = el.clientHeight;
    const osx = Math.max(0, (vw - canvasWRef.current * s) / 2);
    const osy = Math.max(0, (vh - canvasHRef.current * s) / 2);
    return {
      x: (el.scrollLeft - osx + vw / 2) / s,
      y: (el.scrollTop - osy + vh / 2) / s,
    };
  }, []);

  // Update center to parent on scroll end
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const handleScroll = () => {
      isUserScrollingRef.current = true;
      if (scrollEndTimerRef.current) clearTimeout(scrollEndTimerRef.current);
      scrollEndTimerRef.current = setTimeout(() => {
        isUserScrollingRef.current = false;
        const center = getCanvasCenter();
        if (center) {
          lastAppliedCenterRef.current = center;
          onCenterChange?.(center.x, center.y);
        }
      }, 300);
    };
    el.addEventListener('scroll', handleScroll);
    return () => el.removeEventListener('scroll', handleScroll);
  }, [getCanvasCenter, onCenterChange]);

  // Apply external center changes (sync from other tabs)
  useEffect(() => {
    if (centerX === undefined || centerY === undefined) return;
    if (isUserScrollingRef.current || isPanningRef.current || isGestureZoomRef.current) return;

    // Skip if it's the same center we just reported
    if (lastAppliedCenterRef.current &&
        Math.abs(lastAppliedCenterRef.current.x - centerX) < 1 &&
        Math.abs(lastAppliedCenterRef.current.y - centerY) < 1) return;

    const el = scrollRef.current;
    if (!el) return;
    const s = scaleRef.current;
    const vw = el.clientWidth;
    const vh = el.clientHeight;
    const osx = Math.max(0, (vw - canvasWRef.current * s) / 2);
    const osy = Math.max(0, (vh - canvasHRef.current * s) / 2);

    el.scrollLeft = centerX * s + osx - vw / 2;
    el.scrollTop  = centerY * s + osy - vh / 2;
    lastAppliedCenterRef.current = { x: centerX, y: centerY };
  }, [centerX, centerY]);

  // Optimistic local overrides
  const [localOverrides, setLocalOverrides] = useState<Map<string, { x: number; y: number }>>(new Map());

  // Ref so non-passive event handlers can read latest scale without re-registering
  const scaleRef = useRef(scale);
  useEffect(() => { scaleRef.current = scale; }, [scale]);

  const scaleTextRef = useRef<HTMLSpanElement>(null);
  const [isGestureZoom, setIsGestureZoom] = useState(false);
  const gestureTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // RAF id for rendering loop during gesture
  const gestureRafRef = useRef<number | null>(null);

  const lastPinchDist = useRef<number | null>(null);
  const lastPinchMid  = useRef<{ x: number; y: number } | null>(null);

  // Gesture start anchors for jitter-free absolute calculation
  const pinchStartScale = useRef<number>(1);
  const pinchStartDist  = useRef<number>(1);
  const pinchFocalCanvas = useRef<{ x: number; y: number } | null>(null);
  
  // Current values for RAF rendering
  const gestureTarget = useRef({ scale: 1, midX: 0, midY: 0 });

  // Refs so touch/wheel handlers (registered once) can read latest layout values
  const canvasWRef = useRef(0);
  const canvasHRef = useRef(0);

  // Internal flag to skip transitions during manual DOM updates
  const isGestureZoomRef = useRef(false);

  // Separate animation RAF for button zoom
  const scrollAnimRafRef = useRef<number | null>(null);

  // Mouse-drag panning: track at document level so drag outside canvas still works
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!isPanningRef.current || !panStartRef.current) return;
      const el = scrollRef.current;
      if (!el) return;
      const dx = e.clientX - panStartRef.current.x;
      const dy = e.clientY - panStartRef.current.y;
      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) hasPannedRef.current = true;
      el.scrollLeft = panStartRef.current.sl - dx;
      el.scrollTop  = panStartRef.current.st - dy;
    };
    const onUp = () => {
      if (!isPanningRef.current) return;
      isPanningRef.current = false;
      panStartRef.current  = null;
      setIsPanning(false);
    };
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup',   onUp);
    return () => {
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup',   onUp);
    };
  }, []);

  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';

  // Keep track of the viewport size to enable centering small canvases
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const update = () => {
      setViewportSize({ width: el.clientWidth, height: el.clientHeight });
    };
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const wantTypes = useWantTypeStore(state => state.wantTypes);
  const typeMap = useMemo(() => {
    const m = new Map<string, WantTypeListItem>();
    wantTypes.forEach(wt => m.set(wt.name, wt));
    return m;
  }, [wantTypes]);

  const clampScale = (v: number) => Math.max(MIN_SCALE, Math.min(MAX_SCALE, v));

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

  const canvasW = cols * STEP + GAP;
  const canvasH = rows * STEP + GAP;

  // Adjacent empty cells shown as drop-hint during drag
  const recommendationHintCells = useMemo(() => {
    if (!dragWantId) return [];
    const occupied = new Set<string>();
    positionMap.forEach((pos, id) => {
      if (id !== dragWantId) occupied.add(`${pos.x},${pos.y}`);
    });
    const NEIGHBORS = [
      { dx:  1, dy:  0, direction: 'right' as const },
      { dx: -1, dy:  0, direction: 'left'  as const },
      { dx:  0, dy:  1, direction: 'below' as const },
      { dx:  0, dy: -1, direction: 'above' as const },
    ];
    const hints: Array<{ x: number; y: number; direction: 'left' | 'right' | 'above' | 'below' }> = [];
    const seen = new Set<string>();
    positionMap.forEach((pos, id) => {
      if (id === dragWantId) return;
      NEIGHBORS.forEach(({ dx, dy, direction }) => {
        const nx = pos.x + dx;
        const ny = pos.y + dy;
        const key = `${nx},${ny}`;
        if (!seen.has(key) && !occupied.has(key)) {
          hints.push({ x: nx, y: ny, direction });
          seen.add(key);
        }
      });
    });
    return hints;
  }, [dragWantId, positionMap]);

  const { proximity, checkOnDrop, clear: clearProximity, dismiss: dismissProximity, resetDismissed } =
    useFieldMatchProximity({ positionMap, step: STEP, cellSize: CELL_SIZE, originX, originY });

  canvasWRef.current = canvasW;
  canvasHRef.current = canvasH;

  // Offsets used to center the grid if it's smaller than the viewport.
  const offsetX = Math.max(0, (viewportSize.width - canvasW * scale) / 2);
  const offsetY = Math.max(0, (viewportSize.height - canvasH * scale) / 2);

  // Zoom keeping the viewport center fixed in canvas-coordinate space.
  // focalX/focalY: CLIENT (viewport) coords of the fixed point during zoom (default: container center)
  const applyScaleWithCenter = useCallback((newScale: number, focalClientX?: number, focalClientY?: number) => {
    const clamped = clampScale(newScale);
    const el = scrollRef.current;
    if (!el) { onScaleChange?.(clamped); return; }
    const cur = scaleRef.current;
    if (clamped === cur) return;
    scaleRef.current = clamped; // update immediately so rapid gestures read the right value

    const vw = el.clientWidth;
    const vh = el.clientHeight;

    // Convert client coords → container-relative coords
    const rect = el.getBoundingClientRect();
    const fpx = focalClientX !== undefined ? focalClientX - rect.left : vw / 2;
    const fpy = focalClientY !== undefined ? focalClientY - rect.top  : vh / 2;

    const osx = Math.max(0, (vw - canvasW * cur) / 2);
    const osy = Math.max(0, (vh - canvasH * cur) / 2);
    // Canvas-space coordinate under the focal point
    const cx = (el.scrollLeft - osx + fpx) / cur;
    const cy = (el.scrollTop  - osy + fpy) / cur;

    // All zoom paths use the same direct DOM update — no CSS transition, no RAF.
    // Transform, spacer, and scroll are applied atomically in one frame so the
    // focal point never drifts regardless of trigger (gesture / button / R-stick).
    if (scrollAnimRafRef.current !== null) {
      cancelAnimationFrame(scrollAnimRafRef.current);
      scrollAnimRafRef.current = null;
    }
    const nextOsx = Math.max(0, (vw - canvasW * clamped) / 2);
    const nextOsy = Math.max(0, (vh - canvasH * clamped) / 2);
    if (canvasRef.current) {
      canvasRef.current.style.transform = `translate3d(${nextOsx}px, ${nextOsy}px, 0) scale(${clamped})`;
    }
    if (spacerRef.current) {
      spacerRef.current.style.width  = `${canvasW * clamped}px`;
      spacerRef.current.style.height = `${canvasH * clamped}px`;
    }
    el.scrollLeft = cx * clamped + nextOsx - fpx;
    el.scrollTop  = cy * clamped + nextOsy - fpy;
    onScaleChange?.(clamped);
  }, [onScaleChange, canvasW, canvasH]);

  const zoomIn  = () => applyScaleWithCenter(Math.round((scaleRef.current + SCALE_STEP) * 10) / 10);
  const zoomOut = () => applyScaleWithCenter(Math.round((scaleRef.current - SCALE_STEP) * 10) / 10);

  const applyScaleRef = useRef(applyScaleWithCenter);
  useEffect(() => { applyScaleRef.current = applyScaleWithCenter; }, [applyScaleWithCenter]);

  // Gamepad analog: L-stick (axes 0/1) → canvas scroll, R-stick Y (axis 3) → zoom
  useEffect(() => {
    const SCROLL_SPEED = 12; // screen-px per frame at full deflection (~720 px/s at 60 fps)
    const ZOOM_RATE    = 0.02; // scale per frame at full deflection (~1.2/s at 60 fps)
    const DEADZONE     = 0.15;

    let rafHandle: number;

    const poll = () => {
      const gamepads = navigator.getGamepads?.();
      if (gamepads) {
        for (let gi = 0; gi < gamepads.length; gi++) {
          const gp = gamepads[gi];
          if (!gp) continue;

          // L-stick scroll
          const lx = Math.abs(gp.axes[0]) > DEADZONE ? gp.axes[0] : 0;
          const ly = Math.abs(gp.axes[1]) > DEADZONE ? gp.axes[1] : 0;
          const el = scrollRef.current;
          if (el && (lx !== 0 || ly !== 0)) {
            el.scrollLeft += lx * SCROLL_SPEED;
            el.scrollTop  += ly * SCROLL_SPEED;
          }

          // R-stick Y zoom: up (axis < 0) → zoom in, down (axis > 0) → zoom out.
          const ry = gp.axes.length > 3 && Math.abs(gp.axes[3]) > DEADZONE ? gp.axes[3] : 0;
          if (ry !== 0) {
            const newScale = Math.max(MIN_SCALE, Math.min(MAX_SCALE,
              scaleRef.current + (-ry * ZOOM_RATE)
            ));
            if (newScale !== scaleRef.current) applyScaleRef.current(newScale);
          }
        }
      }
      rafHandle = requestAnimationFrame(poll);
    };

    rafHandle = requestAnimationFrame(poll);
    return () => cancelAnimationFrame(rafHandle);
  }, []);

  // Non-passive wheel + pinch listeners (React's synthetic handlers are passive)
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    const onWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        // Zoom: keep viewport center fixed
        e.preventDefault();
        isGestureZoomRef.current = true;
        setIsGestureZoom(true);
        if (gestureTimeoutRef.current) clearTimeout(gestureTimeoutRef.current);
        gestureTimeoutRef.current = setTimeout(() => {
          isGestureZoomRef.current = false;
          setIsGestureZoom(false);
        }, 300);
        const cur = scaleRef.current;
        const factor = e.deltaY < 0 ? 1.05 : 0.95;
        const next = clampScale(cur * factor);
        applyScaleRef.current(next, e.clientX, e.clientY);
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
      const dist = Math.sqrt(dx * dx + dy * dy);
      const midX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const midY = (e.touches[0].clientY + e.touches[1].clientY) / 2;

      lastPinchDist.current = dist;
      lastPinchMid.current  = { x: midX, y: midY };

      const scEl = scrollRef.current;
      const cvEl = canvasRef.current;
      if (scEl && cvEl) {
        isGestureZoomRef.current = true;
        setIsGestureZoom(true);

        const curScale = scaleRef.current;
        const cvRect = cvEl.getBoundingClientRect();
        
        pinchStartScale.current = curScale;
        pinchStartDist.current  = dist;
        pinchFocalCanvas.current = {
          x: (midX - cvRect.left) / curScale,
          y: (midY - cvRect.top)  / curScale,
        };
        
        gestureTarget.current = { scale: curScale, midX, midY };

        // Start high-frequency render loop
        if (gestureRafRef.current) cancelAnimationFrame(gestureRafRef.current);
        const renderLoop = () => {
          const { scale: next, midX: curMidX, midY: curMidY } = gestureTarget.current;
          const targetScEl = scrollRef.current;
          const targetCvEl = canvasRef.current;
          const targetSpEl = spacerRef.current;
          
          if (targetScEl && targetCvEl && targetSpEl) {
            const cW = canvasWRef.current;
            const cH = canvasHRef.current;
            const vw = targetScEl.clientWidth;
            const vh = targetScEl.clientHeight;
            const scRect = targetScEl.getBoundingClientRect();

            const nextOsx = Math.max(0, (vw - cW * next) / 2);
            const nextOsy = Math.max(0, (vh - cH * next) / 2);

            targetCvEl.style.transform = `translate3d(${nextOsx}px, ${nextOsy}px, 0) scale(${next})`;
            targetSpEl.style.width  = `${cW * next}px`;
            targetSpEl.style.height = `${cH * next}px`;

            const focalX = pinchFocalCanvas.current!.x;
            const focalY = pinchFocalCanvas.current!.y;
            targetScEl.scrollLeft = focalX * next + nextOsx - (curMidX - scRect.left);
            targetScEl.scrollTop  = focalY * next + nextOsy - (curMidY - scRect.top);

            // Update UI feedback (toolbar percentage) directly
            if (scaleTextRef.current) {
              scaleTextRef.current.textContent = `${Math.round(next * 100)}%`;
            }
          }
          gestureRafRef.current = requestAnimationFrame(renderLoop);
        };
        gestureRafRef.current = requestAnimationFrame(renderLoop);
      }
    };

    const onTouchMove = (e: TouchEvent) => {
      if (e.touches.length !== 2) return;
      e.preventDefault();
      
      if (gestureTimeoutRef.current) clearTimeout(gestureTimeoutRef.current);

      const dx = e.touches[0].clientX - e.touches[1].clientX;
      const dy = e.touches[0].clientY - e.touches[1].clientY;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const curMidX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const curMidY = (e.touches[0].clientY + e.touches[1].clientY) / 2;

      if (pinchStartDist.current > 0 && pinchFocalCanvas.current) {
        const next = clampScale(pinchStartScale.current * (dist / pinchStartDist.current));
        // Only update the target values; the RAF render loop will pick them up
        gestureTarget.current = { scale: next, midX: curMidX, midY: curMidY };
        scaleRef.current = next;
      }

      lastPinchDist.current = dist;
      lastPinchMid.current  = { x: curMidX, y: curMidY };
    };

    const onTouchEnd = () => {
      if (isGestureZoomRef.current) {
        if (gestureRafRef.current) {
          cancelAnimationFrame(gestureRafRef.current);
          gestureRafRef.current = null;
        }
        
        onScaleChange?.(scaleRef.current);
        
        if (gestureTimeoutRef.current) clearTimeout(gestureTimeoutRef.current);
        gestureTimeoutRef.current = setTimeout(() => {
          isGestureZoomRef.current = false;
          setIsGestureZoom(false);
        }, 150);
      }
      lastPinchDist.current = null;
      lastPinchMid.current = null;
    };

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

  // Scroll once on mount so (0,0) appears at the viewport center.
  // Uses ResizeObserver so we fire only after the container has a real size.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    let scrolled = false;
    const doScroll = () => {
      if (scrolled || !el.clientWidth || !el.clientHeight) return;
      scrolled = true;
      const s = scaleRef.current;
      const vw = el.clientWidth;
      const vh = el.clientHeight;

      // Pixel position of the (0,0) tile's center relative to canvas top-left
      const rawPx = originX * STEP + GAP / 2 + CELL_SIZE / 2;
      const rawPy = originY * STEP + GAP / 2 + CELL_SIZE / 2;

      // Current centering offsets
      const osx = Math.max(0, (vw - canvasW * s) / 2);
      const osy = Math.max(0, (vh - canvasH * s) / 2);

      el.scrollLeft = rawPx * s + osx - vw / 2;
      el.scrollTop  = rawPy * s + osy - vh / 2;
    };
    const ro = new ResizeObserver(doScroll);
    ro.observe(el);
    // Double-RAF as fallback
    requestAnimationFrame(() => requestAnimationFrame(doScroll));
    return () => ro.disconnect();
  }, [originX, originY, canvasW, canvasH]); // re-run if layout bounds change before first scroll

  // Track viewport center of the selected tile for the overlay
  const updateTileCenter = useCallback(() => {
    // Skip heavy overlay position updates during active gestures to maximize smoothness
    if (isGestureZoomRef.current) return;
    
    if (!floatCard || !selectedWant || !scrollRef.current) { setTileCenter(null); return; }
    const id = selectedWant.metadata?.id || selectedWant.id;
    const pos = id ? positionMap.get(id) : undefined;
    if (!pos) { setTileCenter(null); return; }

    const el = scrollRef.current;
    const rect = el.getBoundingClientRect();
    const tileLeft = (pos.x + originX) * STEP + GAP / 2;
    const tileTop  = (pos.y + originY) * STEP + GAP / 2;

    setTileCenter({
      x: rect.left + (tileLeft + CELL_SIZE / 2) * scale + offsetX - el.scrollLeft,
      y: rect.top  + (tileTop  + CELL_SIZE / 2) * scale + offsetY - el.scrollTop,
    });
  }, [floatCard, selectedWant, positionMap, scale, originX, originY, canvasW, canvasH, offsetX, offsetY]);

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

  const handleCanvasMouseDown = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    // Don't start panning when clicking on a tile
    if ((e.target as HTMLElement).closest('[data-want-id]')) return;
    const el = scrollRef.current;
    if (!el) return;
    e.preventDefault(); // prevent text selection during drag
    isPanningRef.current = true;
    hasPannedRef.current = false;
    panStartRef.current  = { x: e.clientX, y: e.clientY, sl: el.scrollLeft, st: el.scrollTop };
    setIsPanning(true);
  }, []);

  const handleCanvasClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    if (dragWantId) return;
    if (hasPannedRef.current) return; // pan drag — not a click
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

  // Scroll canvas so a grid cell is visible in the viewport
  const scrollToCell = useCallback((cx: number, cy: number) => {
    const el = scrollRef.current;
    if (!el) return;
    const s = scaleRef.current;
    const ox = Math.max(0, (el.clientWidth  - canvasWRef.current * s) / 2);
    const oy = Math.max(0, (el.clientHeight - canvasHRef.current * s) / 2);
    const px = (cx + originX) * STEP * s + ox;
    const py = (cy + originY) * STEP * s + oy;
    const padding = CELL_SIZE * s;
    if (px - padding < el.scrollLeft) el.scrollLeft = px - padding;
    if (px + CELL_SIZE * s + padding > el.scrollLeft + el.clientWidth)
      el.scrollLeft = px + CELL_SIZE * s + padding - el.clientWidth;
    if (py - padding < el.scrollTop) el.scrollTop = py - padding;
    if (py + CELL_SIZE * s + padding > el.scrollTop + el.clientHeight)
      el.scrollTop = py + CELL_SIZE * s + padding - el.clientHeight;
  }, [originX, originY]);

  // Keyboard drag imperative handle
  useImperativeHandle(ref, () => ({
    startKeyboardDrag(wantId: string) {
      const pos = positionMap.get(wantId);
      kbDragActiveRef.current = true;
      setDragWantId(wantId);
      setDragOverCell(pos ? { x: pos.x, y: pos.y } : { x: 0, y: 0 });
      resetDismissed();
    },
    moveKeyboardDragCursor(dir: 'up' | 'down' | 'left' | 'right') {
      setDragOverCell(prev => {
        if (!prev) return prev;
        const next = {
          x: prev.x + (dir === 'right' ? 1 : dir === 'left' ? -1 : 0),
          y: prev.y + (dir === 'down'  ? 1 : dir === 'up'   ? -1 : 0),
        };
        scrollToCell(next.x, next.y);
        return next;
      });
    },
    confirmKeyboardDrop() {
      setDragOverCell(cell => {
        setDragWantId(id => {
          if (id && cell && !isCellOccupied(cell.x, cell.y, id)) {
            setLocalOverrides(prev => new Map(prev).set(id, { x: cell.x, y: cell.y }));
            onMoveWant(id, cell.x, cell.y);
            checkOnDrop(id, { x: cell.x, y: cell.y });
          }
          return null;
        });
        return null;
      });
      kbDragActiveRef.current = false;
    },
    cancelKeyboardDrag() {
      setDragWantId(null);
      setDragOverCell(null);
      kbDragActiveRef.current = false;
    },
    isKeyboardDragging() {
      return kbDragActiveRef.current;
    },
  }), [positionMap, isCellOccupied, onMoveWant, checkOnDrop, resetDismissed, scrollToCell]);

  const handleDragOver = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    const isWantMove = e.dataTransfer.types.includes('application/mywant-canvas-id');
    const isTemplateDrop = e.dataTransfer.types.includes('application/mywant-template');

    if (!isWantMove && !isTemplateDrop) return;

    e.preventDefault();
    const { cx, cy } = cellFromEvent(e);
    setDragOverCell({ x: cx, y: cy });
  }, [cellFromEvent]);

  const handleDrop = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    const templateDataRaw = e.dataTransfer.getData('application/mywant-template');
    const wantIdMove = e.dataTransfer.getData('application/mywant-canvas-id');

    if (!templateDataRaw && !wantIdMove) return;

    e.preventDefault();
    const { cx, cy } = cellFromEvent(e);

    if (templateDataRaw) {
      try {
        const t = JSON.parse(templateDataRaw);
        if (t.id && t.type && onTemplateDrop) {
          onTemplateDrop(t.id, t.type, cx, cy);
        }
      } catch (err) {
        console.error('[Canvas] Failed to parse template data:', err);
      }
    } else if (wantIdMove) {
      if (!isCellOccupied(cx, cy, wantIdMove)) {
        setLocalOverrides(prev => new Map(prev).set(wantIdMove, { x: cx, y: cy }));
        onMoveWant(wantIdMove, cx, cy);
      }
      checkOnDrop(wantIdMove, { x: cx, y: cy });
    }

    setDragWantId(null);
    setDragOverCell(null);
  }, [onMoveWant, isCellOccupied, cellFromEvent, checkOnDrop, onTemplateDrop]);

  const handleDragLeave = useCallback((e: React.DragEvent<HTMLDivElement>) => {
    const rect = (e.currentTarget as HTMLDivElement).getBoundingClientRect();
    if (e.clientX < rect.left || e.clientX > rect.right || e.clientY < rect.top || e.clientY > rect.bottom) {
      setDragOverCell(null);
    }
  }, []);

  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;

    const handleTouchDrop = (e: any) => {
      const { template, clientX, clientY } = e.detail;
      if (!template || !onTemplateDrop) return;

      const rect = el.getBoundingClientRect();
      const x = (clientX - rect.left) / scale - originX * STEP - GAP/2;
      const y = (clientY - rect.top) / scale - originY * STEP - GAP/2;
      const cx = Math.floor(x / STEP);
      const cy = Math.floor(y / STEP);

      onTemplateDrop(template.id, template.type, cx, cy);
    };

    el.addEventListener('mywant:template-touch-drop', handleTouchDrop);
    return () => el.removeEventListener('mywant:template-touch-drop', handleTouchDrop);
  }, [onTemplateDrop, scale, originX, originY]);

  return (
    <div
      className="w-full flex-1 relative overflow-hidden"
      style={{ backgroundColor: 'var(--canvas-bg)', minHeight: 0, height: 'calc(100vh - var(--header-height, 56px))' }}
    >
      {/* Zoom toolbar: fixed to the viewport, just inside the header edge.
           Uses --header-height CSS var set by Header component. */}
      <div
        className="z-50 flex items-center gap-1 pointer-events-none select-none"
        style={{
          position: 'fixed',
          left: 8,
          ...(isBottom
            ? { bottom: 'calc(var(--header-height, 56px) + 8px)' }
            : { top: 'calc(var(--header-height, 56px) + 8px)' }),
        }}
      >
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
            ref={scaleTextRef}
            className="pointer-events-auto text-white/70 text-xs font-mono w-12 text-center cursor-pointer hover:text-white transition-colors"
            onClick={() => applyScaleWithCenter(1.0)} title="Reset zoom"
          >{Math.round(scale * 100)}%</span>
          <button
            className="pointer-events-auto w-7 h-7 rounded bg-white/10 hover:bg-white/20 text-white text-lg font-bold flex items-center justify-center transition-colors"
            onClick={zoomIn} title="Zoom in"
          >+</button>
        </div>
      </div>

      {/* Scroll container: absolute fill so height is bounded by the outer div */}
      <div ref={scrollRef} style={{ position: 'absolute', inset: 0, overflow: 'auto' }}>
        {/* Spacer drives scrollbars at scaled size, with min-size to enable centering. */}
        <div ref={spacerRef} style={{
          width: canvasW * scale,
          height: canvasH * scale,
          minWidth: '100%',
          minHeight: '100%',
          position: 'relative',
        }}>
          <div
            ref={canvasRef}
            className="relative select-none"
            style={{
              width: canvasW,
              height: canvasH,
              backgroundColor: 'var(--canvas-bg)',
              backgroundImage: [
                'linear-gradient(var(--canvas-grid) 1px, transparent 1px)',
                'linear-gradient(90deg, var(--canvas-grid) 1px, transparent 1px)',
              ].join(', '),
              backgroundSize: `${STEP}px ${STEP}px`,
              backgroundPosition: `${GAP / 2}px ${GAP / 2}px`,
              cursor: dragWantId || isPanning ? 'grabbing' : 'grab',
              transform: `translate3d(${offsetX}px, ${offsetY}px, 0) scale(${scale})`,
              transformOrigin: '0 0',
              willChange: 'transform',
              position: 'absolute',
              left: 0,
              top: 0,
            }}
            onMouseDown={handleCanvasMouseDown}
            onClick={handleCanvasClick}
            onDragOver={handleDragOver}
            onDrop={handleDrop}
            onDragLeave={handleDragLeave}
            data-want-canvas="true"
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

            {/* Recommendation hint cells: adjacent to other wants during drag */}
            {recommendationHintCells.map(({ x, y, direction }) => {
              const isHorizontal = direction === 'left' || direction === 'right';
              const arrow = { left: '◀', right: '▶', above: '▲', below: '▼' }[direction];
              return (
                <div
                  key={`hint-${x},${y}`}
                  className="absolute pointer-events-none rounded z-0"
                  style={{
                    left: (x + originX) * STEP + GAP / 2,
                    top:  (y + originY) * STEP + GAP / 2,
                    width: CELL_SIZE, height: CELL_SIZE,
                    border: '1.5px dashed rgba(251,146,60,0.55)',
                    backgroundColor: 'rgba(251,146,60,0.08)',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 3,
                  }}
                >
                  <span style={{ fontSize: 16, color: 'rgba(251,146,60,0.65)', lineHeight: 1 }}>{arrow}</span>
                  <span style={{ fontSize: 8, color: 'rgba(251,146,60,0.75)', fontWeight: 600, letterSpacing: '0.04em' }}>
                    {isHorizontal ? 'current' : 'plan / goal'}
                  </span>
                </div>
              );
            })}

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
              const corrRate = correlationHighlights?.get(id) ?? 0;
              const isCorrelated = corrRate > 0;

              return (
                <WantCardFace
                  key={id}
                  typeName={type}
                  displayName={name}
                  category={category}
                  theme={colorMode}
                  context="canvas"
                  iconSize={26}
                  className={classNames(
                    'rounded z-10 transition-transform duration-100',
                    !isDragging && 'hover:scale-[1.06] hover:z-20',
                    isSelected && 'scale-[1.06] z-30',
                    isDragging && 'opacity-40',
                    isCorrelated && 'z-20',
                  )}

                  style={{
                    position: 'absolute',
                    left: (pos.x + originX) * STEP + GAP / 2,
                    top:  (pos.y + originY) * STEP + GAP / 2,
                    width: CELL_SIZE, height: CELL_SIZE,
                    boxShadow: isSelected
                      ? `0 0 0 2px #fff, 0 0 0 4px ${statusColor}, 0 6px 20px rgba(0,0,0,0.7)`
                      : isCorrelated
                      ? `0 0 0 2px rgba(250,204,21,${Math.min(0.4 + corrRate * 0.15, 0.9)}), 0 0 ${12 + corrRate * 4}px rgba(250,204,21,0.5), 0 2px 10px rgba(0,0,0,0.6)`
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
                    resetDismissed();
                  }}
                  onDragEnd={() => {
                    setDragWantId(null);
                    setDragOverCell(null);
                  }}
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

      {/* Field match recommendation bubble */}
      {proximity && (
        <FieldMatchBubble
          proximity={proximity}
          scale={scale}
          offsetX={offsetX}
          offsetY={offsetY}
          scrollLeft={scrollRef.current?.scrollLeft ?? 0}
          scrollTop={scrollRef.current?.scrollTop ?? 0}

          onApply={async (rec: FieldMatchRec) => {
            await fetch('/api/v1/wants/field-match-recommendations/apply', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                source_id: proximity.sourceId,
                target_id: proximity.targetId,
                param_change: rec.param_change,
              }),
            });
          }}
          onDismiss={() => dismissProximity(proximity.sourceId, proximity.targetId, proximity.direction)}
          getContainerRect={() => scrollRef.current?.getBoundingClientRect() ?? null}
        />
      )}
    </div>
  );
});
