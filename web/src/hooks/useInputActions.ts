import { useEffect, useRef } from 'react';

// ─── Public types ─────────────────────────────────────────────────────────────

export type NavigationDirection = 'up' | 'down' | 'left' | 'right' | 'home' | 'end';

export interface UseInputActionsOptions {
  /** Called when a directional navigation input is received */
  onNavigate?: (direction: NavigationDirection) => void;
  /** Called on Enter key or Gamepad A button (index 0) */
  onConfirm?: () => void;
  /** Called on Escape key or Gamepad B button (index 1) */
  onCancel?: () => void;
  /** Called on Space key or Gamepad X button (index 2) */
  onToggle?: () => void;
  /** Called on Alt+Space or Gamepad Select button (index 8) */
  onMenuToggle?: () => void;
  /**
   * Called on Shift+Space or Gamepad Start button (index 9).
   * Intended for a context-menu / right-click equivalent overlay.
   */
  onContextMenu?: () => void;
  /**
   * Called on the `y` key or Gamepad Y button (index 3 / Triangle).
   * Intended for entering header button selection mode.
   */
  onYButton?: () => void;
  /**
   * Called on Tab key or Gamepad R bumper (index 5).
   * When no callback is provided the R bumper simulates a Tab keypress
   * (moves DOM focus to the next focusable element).
   */
  onTabForward?: () => void;
  /**
   * Called on Shift+Tab or Gamepad L bumper (index 4).
   * When no callback is provided the L bumper simulates Shift+Tab
   * (moves DOM focus to the previous focusable element).
   */
  onTabBackward?: () => void;
  enabled?: boolean;
  /** Skip if an <input>/<textarea>/contentEditable is focused. Default: true */
  ignoreWhenInputFocused?: boolean;
  /** Skip if focus is inside a [data-sidebar="true"] element. Default: true */
  ignoreWhenInSidebar?: boolean;
  /**
   * When true, this handler intercepts all input before other useInputActions
   * instances (keyboard: capture phase + stopImmediatePropagation; gamepad:
   * exclusive dispatch).  Use for modal/menu navigation that must take priority
   * over page-level handlers.
   */
  captureInput?: boolean;
  /**
   * When true, only the gamepad listener is registered — keyboard events are
   * ignored entirely.  Use when a component already has its own keydown handler
   * for keyboard behaviour but still needs gamepad equivalents.
   */
  gamepadOnly?: boolean;
}

// ─── Gamepad singleton ────────────────────────────────────────────────────────

type GamepadActionType =
  | NavigationDirection
  | 'confirm'
  | 'cancel'
  | 'toggle'
  | 'menu-toggle'
  | 'context-menu'
  | 'tab-forward'
  | 'tab-backward'
  | 'y-button';

type GamepadActionListener = (action: GamepadActionType) => void;

// Repeat timing constants (ms) – mimics OS key-repeat behaviour
const INITIAL_REPEAT_DELAY = 400;
const REPEAT_INTERVAL = 120;
const AXIS_DEADZONE = 0.5;

// Standard Gamepad API button indices
const BUTTON_MAP: Readonly<Record<number, GamepadActionType>> = {
  0: 'confirm',       // A / Cross
  1: 'cancel',        // B / Circle
  2: 'toggle',        // X / Square
  3: 'y-button',      // Y / Triangle
  4: 'tab-backward',  // L Bumper (LB / L1)
  5: 'tab-forward',   // R Bumper (RB / R1)
  8: 'menu-toggle',   // Select / Back / View / Share
  9: 'context-menu',  // Start / Options / Menu
  12: 'up',           // D-pad Up
  13: 'down',         // D-pad Down
  14: 'left',         // D-pad Left
  15: 'right',        // D-pad Right
};

const NAV_ACTIONS = new Set<GamepadActionType>(['up', 'down', 'left', 'right']);

interface TrackState {
  pressed: boolean;
  repeatTimeout: ReturnType<typeof setTimeout> | null;
  repeatInterval: ReturnType<typeof setInterval> | null;
}

// Module-level singleton — only one RAF loop runs regardless of how many hook
// instances are active.
const _listeners = new Set<GamepadActionListener>();
// When set, only this listener receives gamepad events (all others are bypassed).
let _captureListener: GamepadActionListener | null = null;
let _rafHandle: number | null = null;
const _trackStates = new Map<string, TrackState>();

// Flag used inside _emit to detect whether a listener consumed a tab action
// with an explicit custom callback.  Prevents _focusNext from running when a
// consumer provides its own onTabForward / onTabBackward handler.
let _tabActionConsumed = false;

function _emit(action: GamepadActionType): void {
  if (_captureListener) {
    _captureListener(action);
    return;
  }

  // For tab-like actions, fire all listeners first (so custom callbacks can
  // set _tabActionConsumed), then — if no listener consumed it — do the
  // default DOM-focus simulation exactly once.
  if (action === 'tab-forward' || action === 'tab-backward') {
    _tabActionConsumed = false;
    _listeners.forEach(fn => fn(action));
    if (!_tabActionConsumed) {
      _focusNext(action === 'tab-backward');
    }
    return;
  }

  _listeners.forEach(fn => fn(action));
}

function _getOrCreate(key: string): TrackState {
  if (!_trackStates.has(key)) {
    _trackStates.set(key, { pressed: false, repeatTimeout: null, repeatInterval: null });
  }
  return _trackStates.get(key)!;
}

function _beginRepeat(key: string, action: GamepadActionType): void {
  if (!NAV_ACTIONS.has(action)) return;
  const state = _trackStates.get(key);
  if (!state) return;
  state.repeatTimeout = setTimeout(() => {
    if (!_trackStates.get(key)?.pressed) return;
    _emit(action);
    state.repeatInterval = setInterval(() => {
      if (!_trackStates.get(key)?.pressed) {
        clearInterval(state.repeatInterval!);
        state.repeatInterval = null;
        return;
      }
      _emit(action);
    }, REPEAT_INTERVAL);
  }, INITIAL_REPEAT_DELAY);
}

function _endRepeat(key: string): void {
  const state = _trackStates.get(key);
  if (!state) return;
  if (state.repeatTimeout !== null) { clearTimeout(state.repeatTimeout); state.repeatTimeout = null; }
  if (state.repeatInterval !== null) { clearInterval(state.repeatInterval); state.repeatInterval = null; }
}

function _pollGamepads(): void {
  const gamepads = navigator.getGamepads?.();
  if (gamepads) {
    for (let gi = 0; gi < gamepads.length; gi++) {
      const gp = gamepads[gi];
      if (!gp) continue;

      // Mapped buttons
      for (const [btnIdxStr, action] of Object.entries(BUTTON_MAP)) {
        const bi = Number(btnIdxStr);
        if (bi >= gp.buttons.length) continue;
        const key = `${gi}:b${bi}`;
        const state = _getOrCreate(key);
        const pressed = gp.buttons[bi].pressed;

        if (pressed && !state.pressed) {
          state.pressed = true;
          _emit(action);
          _beginRepeat(key, action);
        } else if (!pressed && state.pressed) {
          state.pressed = false;
          _endRepeat(key);
        }
      }

      // Left analog stick — supplements / replaces D-pad on controllers without
      // discrete D-pad buttons.
      const axisBindings: [axisIdx: number, sign: -1 | 1, action: NavigationDirection][] = [
        [0, -1, 'left'],
        [0, +1, 'right'],
        [1, -1, 'up'],
        [1, +1, 'down'],
      ];
      for (const [axisIdx, sign, action] of axisBindings) {
        if (axisIdx >= gp.axes.length) continue;
        const key = `${gi}:a${axisIdx}:${sign > 0 ? 'pos' : 'neg'}`;
        const state = _getOrCreate(key);
        const value = gp.axes[axisIdx];
        const active = sign < 0 ? value < -AXIS_DEADZONE : value > AXIS_DEADZONE;

        if (active && !state.pressed) {
          state.pressed = true;
          _emit(action);
          _beginRepeat(key, action);
        } else if (!active && state.pressed) {
          state.pressed = false;
          _endRepeat(key);
        }
      }
    }
  }

  _rafHandle = requestAnimationFrame(_pollGamepads);
}

function _startPolling(): void {
  if (_rafHandle !== null) return;
  _rafHandle = requestAnimationFrame(_pollGamepads);
}

function _stopPolling(): void {
  if (_rafHandle !== null) {
    cancelAnimationFrame(_rafHandle);
    _rafHandle = null;
  }
  _trackStates.forEach((_, key) => _endRepeat(key));
  _trackStates.clear();
}

function _registerListener(fn: GamepadActionListener): void {
  _listeners.add(fn);
  if (_listeners.size === 1) _startPolling();
}

function _unregisterListener(fn: GamepadActionListener): void {
  _listeners.delete(fn);
  if (_listeners.size === 0) _stopPolling();
}

// ─── Guard helpers ────────────────────────────────────────────────────────────

function _isInputFocused(): boolean {
  const el = document.activeElement as HTMLElement | null;
  return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
}

function _isInSidebar(target?: HTMLElement | null): boolean {
  const el = target ?? (document.activeElement as HTMLElement | null);
  return !!el?.closest('[data-sidebar="true"]');
}

// ─── Tab focus simulation ─────────────────────────────────────────────────────
// Used by gamepad L/R bumper buttons to replicate browser Tab / Shift+Tab
// behaviour when no explicit onTabForward/onTabBackward callback is provided.
// Called at most ONCE per button press (guarded by _tabActionConsumed flag in
// _emit, so multiple registered listeners never multiply this call).

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function _focusNext(reverse: boolean): void {
  // Use getBoundingClientRect for visibility check — more reliable than
  // offsetParent which returns null for position:fixed ancestors.
  const candidates = Array.from(
    document.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)
  ).filter(el => {
    if (el.closest('[aria-hidden="true"]')) return false;
    const rect = el.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0;
  });

  if (candidates.length === 0) return;

  const active = document.activeElement as HTMLElement | null;
  const activeIdx = active ? candidates.indexOf(active) : -1;
  const nextIdx = reverse
    ? (activeIdx <= 0 ? candidates.length - 1 : activeIdx - 1)
    : (activeIdx < 0 ? 0 : (activeIdx + 1) % candidates.length);

  const target = candidates[nextIdx];
  if (target) {
    target.focus();
    target.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' });
  }
}

// ─── Hook ─────────────────────────────────────────────────────────────────────

/**
 * Unified keyboard + Gamepad API input handler.
 *
 * Keyboard mapping:
 *   Arrow keys   → onNavigate (up / down / left / right)
 *   Home / End   → onNavigate (home / end)
 *   Enter        → onConfirm
 *   Escape       → onCancel
 *   Space        → onToggle
 *   Alt+Space    → onMenuToggle
 *   Shift+Space  → onContextMenu
 *   Tab          → onTabForward  (no preventDefault — browser focus also moves)
 *   Shift+Tab    → onTabBackward (no preventDefault)
 *
 * Gamepad mapping (Standard Gamepad layout):
 *   D-pad / Left stick  → onNavigate
 *   A (0)               → onConfirm
 *   B (1)               → onCancel
 *   X (2)               → onToggle
 *   L Bumper (4)        → onTabBackward / simulate Shift+Tab
 *   R Bumper (5)        → onTabForward  / simulate Tab
 *   Select (8)          → onMenuToggle
 *   Start (9)           → onContextMenu
 *
 * Navigation inputs have key-repeat behaviour (400 ms initial, 120 ms repeat).
 *
 * Set captureInput: true to give this handler exclusive priority — keyboard
 * events are intercepted in the capture phase (stopImmediatePropagation) and
 * all gamepad events go only to this handler while active.
 */
export function useInputActions({
  onNavigate,
  onConfirm,
  onCancel,
  onToggle,
  onMenuToggle,
  onContextMenu,
  onYButton,
  onTabForward,
  onTabBackward,
  enabled = true,
  ignoreWhenInputFocused = true,
  ignoreWhenInSidebar = true,
  captureInput = false,
  gamepadOnly = false,
}: UseInputActionsOptions): void {
  // Refs let us update callbacks without re-subscribing to events.
  const onNavigateRef    = useRef(onNavigate);
  const onConfirmRef     = useRef(onConfirm);
  const onCancelRef      = useRef(onCancel);
  const onToggleRef      = useRef(onToggle);
  const onMenuToggleRef  = useRef(onMenuToggle);
  const onContextMenuRef = useRef(onContextMenu);
  const onYButtonRef     = useRef(onYButton);
  const onTabForwardRef  = useRef(onTabForward);
  const onTabBackwardRef = useRef(onTabBackward);
  const enabledRef       = useRef(enabled);

  // Keep refs current on every render.
  onNavigateRef.current    = onNavigate;
  onConfirmRef.current     = onConfirm;
  onCancelRef.current      = onCancel;
  onToggleRef.current      = onToggle;
  onMenuToggleRef.current  = onMenuToggle;
  onContextMenuRef.current = onContextMenu;
  onYButtonRef.current     = onYButton;
  onTabForwardRef.current  = onTabForward;
  onTabBackwardRef.current = onTabBackward;
  enabledRef.current       = enabled;

  // ── Normal (bubble-phase) keyboard handler — active when captureInput is false ──
  useEffect(() => {
    if (!enabled || captureInput || gamepadOnly) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (!enabledRef.current) return;
      const target = e.target as HTMLElement;
      if (ignoreWhenInputFocused && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) return;
      if (ignoreWhenInSidebar && target.closest('[data-sidebar="true"]')) return;

      switch (e.key) {
        // Navigation — always preventDefault to suppress page-scroll
        case 'ArrowUp':    e.preventDefault(); onNavigateRef.current?.('up');    break;
        case 'ArrowDown':  e.preventDefault(); onNavigateRef.current?.('down');  break;
        case 'ArrowLeft':  e.preventDefault(); onNavigateRef.current?.('left');  break;
        case 'ArrowRight': e.preventDefault(); onNavigateRef.current?.('right'); break;
        case 'Home':       e.preventDefault(); onNavigateRef.current?.('home');  break;
        case 'End':        e.preventDefault(); onNavigateRef.current?.('end');   break;
        // Enter / Escape — only preventDefault when we handle them
        case 'Enter':
          if (onConfirmRef.current) { e.preventDefault(); onConfirmRef.current(); }
          break;
        case 'Escape':
          if (onCancelRef.current) { e.preventDefault(); onCancelRef.current(); }
          break;
        // Space variants — priority: Alt > Shift > plain
        case ' ':
          if (e.altKey) {
            if (onMenuToggleRef.current) { e.preventDefault(); e.stopImmediatePropagation(); onMenuToggleRef.current(); }
          } else if (e.shiftKey) {
            if (onContextMenuRef.current) { e.preventDefault(); onContextMenuRef.current(); }
          } else {
            if (onToggleRef.current) { e.preventDefault(); onToggleRef.current(); }
          }
          break;
        // Tab / Shift+Tab — no preventDefault so browser focus still moves
        case 'Tab':
          if (e.shiftKey) {
            onTabBackwardRef.current?.();
          } else {
            onTabForwardRef.current?.();
          }
          break;
        case 'y':
        case 'Y':
          if (!e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
            if (onYButtonRef.current) { e.preventDefault(); onYButtonRef.current(); }
          }
          break;
        default:
          return;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [enabled, captureInput, gamepadOnly, ignoreWhenInputFocused, ignoreWhenInSidebar]);

  // ── Capture-phase keyboard handler — active when captureInput is true ────────
  useEffect(() => {
    if (!enabled || !captureInput || gamepadOnly) return;

    const handleKeyDownCapture = (e: KeyboardEvent) => {
      if (!enabledRef.current) return;
      const target = e.target as HTMLElement;
      if (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable) return;

      let handled = false;
      switch (e.key) {
        case 'ArrowUp':    handled = true; onNavigateRef.current?.('up');    break;
        case 'ArrowDown':  handled = true; onNavigateRef.current?.('down');  break;
        case 'ArrowLeft':  handled = true; onNavigateRef.current?.('left');  break;
        case 'ArrowRight': handled = true; onNavigateRef.current?.('right'); break;
        case 'Home':       handled = true; onNavigateRef.current?.('home');  break;
        case 'End':        handled = true; onNavigateRef.current?.('end');   break;
        case 'Enter':  if (onConfirmRef.current)  { handled = true; onConfirmRef.current();  } break;
        case 'Escape': if (onCancelRef.current)   { handled = true; onCancelRef.current();   } break;
        case ' ':
          if (e.altKey) {
            if (onMenuToggleRef.current)  { handled = true; onMenuToggleRef.current(); }
          } else if (e.shiftKey) {
            if (onContextMenuRef.current) { handled = true; onContextMenuRef.current(); }
          } else {
            if (onToggleRef.current)      { handled = true; onToggleRef.current(); }
          }
          break;
        case 'Tab':
          if (e.shiftKey) { onTabBackwardRef.current?.(); }
          else            { onTabForwardRef.current?.();  }
          // Never stopImmediatePropagation for Tab — browser focus must still move
          return;
        case 'y':
        case 'Y':
          if (!e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey) {
            if (onYButtonRef.current) { handled = true; onYButtonRef.current(); }
          }
          break;
        default:
          return;
      }

      if (handled) {
        e.preventDefault();
        e.stopImmediatePropagation();
      }
    };

    window.addEventListener('keydown', handleKeyDownCapture, true);
    return () => window.removeEventListener('keydown', handleKeyDownCapture, true);
  }, [enabled, captureInput, gamepadOnly, ignoreWhenInputFocused, ignoreWhenInSidebar]);

  // ── Gamepad ───────────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!enabled) return;

    const handleGamepadAction = (action: GamepadActionType) => {
      if (!enabledRef.current) return;
      if (!captureInput) {
        if (ignoreWhenInputFocused && _isInputFocused()) return;
        if (ignoreWhenInSidebar && _isInSidebar()) return;
      }

      switch (action) {
        case 'up':
        case 'down':
        case 'left':
        case 'right':
        case 'home':
        case 'end':
          onNavigateRef.current?.(action);
          break;
        case 'confirm':      onConfirmRef.current?.();     break;
        case 'cancel':       onCancelRef.current?.();      break;
        case 'toggle':       onToggleRef.current?.();      break;
        case 'menu-toggle':  onMenuToggleRef.current?.();  break;
        case 'context-menu': onContextMenuRef.current?.(); break;
        case 'y-button':     onYButtonRef.current?.();     break;
        case 'tab-forward':
          if (onTabForwardRef.current) { _tabActionConsumed = true; onTabForwardRef.current(); }
          break;
        case 'tab-backward':
          if (onTabBackwardRef.current) { _tabActionConsumed = true; onTabBackwardRef.current(); }
          break;
      }
    };

    _registerListener(handleGamepadAction);
    if (captureInput) _captureListener = handleGamepadAction;

    return () => {
      _unregisterListener(handleGamepadAction);
      if (_captureListener === handleGamepadAction) _captureListener = null;
    };
  }, [enabled, captureInput, gamepadOnly, ignoreWhenInputFocused, ignoreWhenInSidebar]);
}
