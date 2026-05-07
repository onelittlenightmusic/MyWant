import React from 'react';
import { Want } from '@/types/want';

// Props passed to every want card plugin's ContentSection.
// Keep this minimal — derive extra values inside each plugin from `want`.
export interface WantCardPluginProps {
  want: Want;
  isChild: boolean;
  isControl: boolean;
  isFocused: boolean;
  isSelectMode: boolean;
  onView: (want: Want) => void;
  onViewResults?: (want: Want) => void;
  onSliderActiveChange?: (active: boolean) => void;
  /** True when keyboard/gamepad has drilled into this control's inner focus */
  isInnerFocused?: boolean;
  /** Called by the plugin to exit inner focus back to the card */
  onExitInnerFocus?: () => void;
}

export interface WantCardPlugin {
  /** One or more want type names this plugin handles */
  types: string[];
  /** Type-specific content rendered in the card body */
  ContentSection: React.ComponentType<WantCardPluginProps>;
  /** When true, suppresses the default final result JSON display */
  hideFinalResult?: boolean;
}

const pluginRegistry = new Map<string, WantCardPlugin>();
const registryListeners = new Set<() => void>();

export function registerWantCardPlugin(plugin: WantCardPlugin): void {
  for (const type of plugin.types) {
    pluginRegistry.set(type, plugin);
  }
  registryListeners.forEach(fn => fn());
}

export function getWantCardPlugin(type: string): WantCardPlugin | undefined {
  return pluginRegistry.get(type);
}

export function onPluginRegistered(cb: () => void): () => void {
  registryListeners.add(cb);
  return () => registryListeners.delete(cb);
}
