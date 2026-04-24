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
}

export interface WantCardPlugin {
  /** One or more want type names this plugin handles */
  types: string[];
  /** Type-specific content rendered in the card body */
  ContentSection: React.ComponentType<WantCardPluginProps>;
}

const pluginRegistry = new Map<string, WantCardPlugin>();

export function registerWantCardPlugin(plugin: WantCardPlugin): void {
  for (const type of plugin.types) {
    pluginRegistry.set(type, plugin);
  }
}

export function getWantCardPlugin(type: string): WantCardPlugin | undefined {
  return pluginRegistry.get(type);
}
