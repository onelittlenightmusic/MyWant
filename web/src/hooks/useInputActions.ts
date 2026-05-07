import { useEffect, useRef } from 'react';

// ─── Public types ─────────────────────────────────────────────────────────────

export type NavigationDirection = 'up' | 'down' | 'left' | 'right' | 'home' | 'end';

export interface UseInputActionsOptions {
  /** Called when a directional navigation input is received */
  onNavigate?: (direction: NavigationDirection) => void;
  /** Called on Enter key or Gamepad A button */
  onConfirm?: () => void;
  /** Called on Escape key or Gamepad B button */
  onCancel?: () => void;
  /** Called on Space key or Gamepad X button */
  onToggle?: () => void;
  enabled?: boolean;
  /** Skip if an <input>/<textarea>/contentEditable is focused. Default: true */
  ignoreWhenInputFocused?: boolean;
  /** Skip if focus is inside a [data-sidebar="true"] element. Default: true */
  ignoreWhenInSidebar?: boolean;
}

// ─── Gamepad singleton ────────────────────────────────────────────────────────

type GamepadActionType = NavigationDirection | 'confirm' | 'cancel' | 'toggle';
type GamepadActionListener = (action: GamepadActionType) => void;

// Repeat timing constants (ms) – mimics OS key-repeat behaviour
const INITIAL_REPEAT_DELAY = 400;
const REPEAT_INTERVAL = 120;
const AXIS_DEADZONE = 0.5;

// Standard Gamepad API button indices
const BUTTON_MAP: Readonly<Record<number, GamepadActionType>> = {
  0: 'confirm',  // A / Cross
  1: 'cancel',   // B / Circle
  2: 'toggle',   // X / Square
  12: 'up',      // D-pad Up
  13: 'down',    // D-pad Down
  14: 'left',    // D-pad Left
  15: 'right',   // D-pad Right
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
let _rafHandle: number | null = null;
const _trackStates = new Map<string, TrackState>();

function _emit(action: GamepadActionType): void {
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

// ─── Hook ─────────────────────────────────────────────────────────────────────

/**
 * Unified keyboard + Gamepad API input handler.
 *
 * Keyboard mapping:
 *   Arrow keys → onNavigate   (up / down / left / right)
 *   Home / End → onNavigate   (home / end)
 *   Enter      → onConfirm
 *   Escape     → onCancel
 *   Space      → onToggle
 *
 * Gamepad mapping (Standard Gamepad layout):
 *   D-pad / Left analog stick → onNavigate
 *   A button (index 0)        → onConfirm
 *   B button (index 1)        → onCancel
 *   X button (index 2)        → onToggle
 *
 * Navigation inputs have key-repeat behaviour: first event fires immediately,
 * then repeats after 400 ms at 120 ms intervals while held.
 */
export function useInputActions({
  onNavigate,
  onConfirm,
  onCancel,
  onToggle,
  enabled = true,
  ignoreWhenInputFocused = true,
  ignoreWhenInSidebar = true,
}: UseInputActionsOptions): void {
  // Refs let us update callbacks without re-subscribing to events.
  const onNavigateRef = useRef(onNavigate);
  const onConfirmRef  = useRef(onConfirm);
  const onCancelRef   = useRef(onCancel);
  const onToggleRef   = useRef(onToggle);
  const enabledRef    = useRef(enabled);

  // Keep refs current on every render (no deps needed — runs every render).
  onNavigateRef.current = onNavigate;
  onConfirmRef.current  = onConfirm;
  onCancelRef.current   = onCancel;
  onToggleRef.current   = onToggle;
  enabledRef.current    = enabled;

  // ── Keyboard ──────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!enabled) return;

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
        // Action keys — only preventDefault when we actually handle them
        case 'Enter':
          if (onConfirmRef.current) { e.preventDefault(); onConfirmRef.current(); }
          break;
        case 'Escape':
          if (onCancelRef.current) { e.preventDefault(); onCancelRef.current(); }
          break;
        case ' ':
          if (onToggleRef.current) { e.preventDefault(); onToggleRef.current(); }
          break;
        default:
          return;
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [enabled, ignoreWhenInputFocused, ignoreWhenInSidebar]);

  // ── Gamepad ───────────────────────────────────────────────────────────────
  useEffect(() => {
    if (!enabled) return;

    const handleGamepadAction = (action: GamepadActionType) => {
      if (!enabledRef.current) return;
      if (ignoreWhenInputFocused && _isInputFocused()) return;
      if (ignoreWhenInSidebar && _isInSidebar()) return;

      switch (action) {
        case 'up':
        case 'down':
        case 'left':
        case 'right':
        case 'home':
        case 'end':
          onNavigateRef.current?.(action);
          break;
        case 'confirm': onConfirmRef.current?.();  break;
        case 'cancel':  onCancelRef.current?.();   break;
        case 'toggle':  onToggleRef.current?.();   break;
      }
    };

    _registerListener(handleGamepadAction);
    return () => _unregisterListener(handleGamepadAction);
  }, [enabled, ignoreWhenInputFocused, ignoreWhenInSidebar]);
}
