import React from 'react';
import { WantCardPluginProps, registerWantCardPlugin } from '../registry';

const RpgStageViewContentSection: React.FC<WantCardPluginProps> = ({
  want, isChild, isControl, isFocused,
}) => {
  const scene = (want.state?.current?.scene as string) || '';
  const stageId = (want.state?.current?.stage as string) || '';

  const mt = (isChild || (isControl && !isFocused)) ? 'mt-2' : 'mt-3';

  if (!scene) {
    return (
      <div className={`${mt} rounded-lg bg-gray-900 text-gray-500 text-xs font-mono px-3 py-2`}>
        観測中…
      </div>
    );
  }

  return (
    <div className={`${mt} rounded-lg overflow-hidden`}
      style={{ background: '#0d1117' }}>

      {/* terminal title bar */}
      <div className="flex items-center gap-1.5 px-3 py-1.5"
        style={{ background: '#161b22', borderBottom: '1px solid #30363d' }}>
        <span className="w-2.5 h-2.5 rounded-full bg-red-500 opacity-80" />
        <span className="w-2.5 h-2.5 rounded-full bg-yellow-400 opacity-80" />
        <span className="w-2.5 h-2.5 rounded-full bg-green-500 opacity-80" />
        <span className="ml-2 text-xs font-mono"
          style={{ color: '#8b949e' }}>
          {stageId || 'rpg-stage-view'}
        </span>
        <span className="ml-auto flex items-center gap-1">
          <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
          <span className="text-xs" style={{ color: '#3fb950' }}>live</span>
        </span>
      </div>

      {/* scene content */}
      <pre
        className="text-xs leading-tight px-3 py-2 overflow-x-auto"
        style={{
          color: '#e6edf3',
          fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", ui-monospace, monospace',
          whiteSpace: 'pre',
          margin: 0,
        }}
      >
        {scene}
      </pre>
    </div>
  );
};

registerWantCardPlugin({
  types: ['rpg_stage_view'],
  ContentSection: RpgStageViewContentSection,
});
