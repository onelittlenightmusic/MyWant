/** Default polling interval (ms) used across Dashboard and Want Detail views. */
export const POLLING_INTERVAL_MS = 250;

/** Available polling interval presets for the debug settings UI. */
export const POLLING_PRESETS = [
  { label: 'Default (250 ms)', value: 250 },
  { label: 'Slow (1 s)',       value: 1000 },
] as const;
