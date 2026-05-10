import React from 'react';

interface ObjectResultDisplayProps {
  data: Record<string, unknown>;
  maxRows?: number;
  size?: 'compact' | 'normal';
}

export const formatObjectValue = (v: unknown, size: 'compact' | 'normal'): { text: string; muted: boolean } => {
  if (v === null || v === undefined) return { text: '—', muted: true };
  if (typeof v === 'boolean') return { text: String(v), muted: false };
  if (typeof v === 'number') return { text: String(v), muted: false };
  if (typeof v === 'string') return { text: v, muted: false };
  if (Array.isArray(v)) {
    if (v.length === 0) return { text: '[]', muted: true };
    return { text: `[${v.length} items]`, muted: true };
  }
  const json = JSON.stringify(v);
  const limit = size === 'compact' ? 30 : 60;
  return { text: json.length > limit ? json.slice(0, limit) + '…' : json, muted: true };
};

export const isPlainObject = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === 'object' && !Array.isArray(v);

export const ObjectResultDisplay: React.FC<ObjectResultDisplayProps> = ({
  data,
  maxRows = 5,
  size = 'normal',
}) => {
  const entries = Object.entries(data);
  const visibleEntries = entries.slice(0, maxRows);
  const remaining = entries.length - maxRows;

  const fontClass = size === 'compact'
    ? 'text-[0.5rem] sm:text-[0.55rem]'
    : 'text-xs sm:text-sm';
  const cellPad = size === 'compact' ? 'px-1.5 py-0.5' : 'px-2 py-1';
  const keyMaxWidth = size === 'compact' ? 'max-w-[6rem]' : 'max-w-[10rem]';
  const valMaxWidth = size === 'compact' ? 'max-w-[12rem]' : 'max-w-[20rem]';

  return (
    <div className="overflow-x-auto rounded border border-green-700/40 bg-gray-900/80">
      <table className={`${fontClass} font-mono w-full border-collapse`}>
        <tbody>
          {visibleEntries.map(([key, value], i) => {
            const { text, muted } = formatObjectValue(value, size);
            return (
              <tr key={key} className={i % 2 !== 0 ? 'bg-white/5' : ''}>
                <td className={`${cellPad} text-green-400/70 font-semibold whitespace-nowrap ${keyMaxWidth} truncate align-top`}>
                  {key}
                </td>
                <td className={`${cellPad} ${muted ? 'text-gray-500 italic' : 'text-gray-300'} ${valMaxWidth} truncate whitespace-nowrap`}>
                  {text}
                </td>
              </tr>
            );
          })}
          {remaining > 0 && (
            <tr>
              <td colSpan={2} className={`${cellPad} text-gray-500 italic text-center`}>
                + {remaining} more fields
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
};
