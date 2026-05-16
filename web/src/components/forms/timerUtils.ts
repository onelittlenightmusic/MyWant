// Shared constants, pure functions, and types for timer UI components.
// Used by TimerEveryDial, TimerClockFace, TimerPickerContent, TimerCardPlugin.

export type TimerMode = 'every' | 'at';
export type ClockMode = 'hour' | 'minute';

// ── Every mode ────────────────────────────────────────────────────────────────

export const EVERY_PRESETS = ['10s', '30s', '1m', '5m', '10m', '30m', '1h', '6h', '1d'];

export const toSeconds = (s: string): number => {
  if (s.endsWith('d')) return parseInt(s) * 86400;
  if (s.endsWith('h')) return parseInt(s) * 3600;
  if (s.endsWith('m')) return parseInt(s) * 60;
  if (s.endsWith('s')) return parseInt(s);
  return parseInt(s);
};

export const START_DEG = -70;
export const ARC_DEG = 270;
export const CX = 70, CY = 70;
export const R_FACE = 56, R_TICK_E = 42, R_LABEL_E = 57;

const LOG_VALS = EVERY_PRESETS.map(p => Math.log(toSeconds(p)));
const LOG_MIN = LOG_VALS[0];
const LOG_RANGE = LOG_VALS[LOG_VALS.length - 1] - LOG_MIN;

export const angleForEvery = (i: number): number =>
  (START_DEG + ((LOG_VALS[i] - LOG_MIN) / LOG_RANGE) * ARC_DEG) * (Math.PI / 180);

// ── At mode ───────────────────────────────────────────────────────────────────

export const HOUR_RING  = [12, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11];
export const MINUTE_RING = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55];
export const WEEKDAYS    = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'];
export const WEEKDAY_VALS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

export const R_AT_TICK = 44;
export const R_AT_LABEL = 57;

export const clockPos = (i: number, count: number, r: number) => {
  const angle = ((i / count) * 360 - 90) * (Math.PI / 180);
  return { x: CX + r * Math.cos(angle), y: CY + r * Math.sin(angle) };
};

export const parseAt = (at: string): { h24: number; min: number } => {
  const parts = (at || '').split(':').map(Number);
  const h24 = isNaN(parts[0]) ? 9 : Math.max(0, Math.min(23, parts[0]));
  const min  = isNaN(parts[1]) ? 0 : Math.max(0, Math.min(59, parts[1]));
  return { h24, min };
};

export const formatAt = (h24: number, min: number): string =>
  `${String(h24).padStart(2, '0')}:${String(min).padStart(2, '0')}`;

export const to12h = (h24: number): { isPM: boolean; h12: number } => ({
  isPM: h24 >= 12,
  h12: h24 % 12 === 0 ? 12 : h24 % 12,
});
