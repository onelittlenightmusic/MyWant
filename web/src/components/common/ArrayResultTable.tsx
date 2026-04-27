import React from 'react';

interface ArrayResultTableProps {
  data: Record<string, unknown>[];
  maxRows?: number;
  /** compact: card-size font. normal: sidebar-size font. */
  size?: 'compact' | 'normal';
}

export const ArrayResultTable: React.FC<ArrayResultTableProps> = ({
  data,
  maxRows = 5,
  size = 'normal',
}) => {
  const columns = Object.keys(data[0] ?? {});
  const rows = data.slice(0, maxRows);
  const remaining = data.length - maxRows;

  const fontClass = size === 'compact'
    ? 'text-[0.5rem] sm:text-[0.55rem]'
    : 'text-xs sm:text-sm';
  const cellPad = size === 'compact' ? 'px-1.5 py-0.5' : 'px-2 py-1';
  const maxCellWidth = size === 'compact' ? 'max-w-[10rem]' : 'max-w-[16rem]';

  return (
    <div className="overflow-x-auto rounded border border-green-700/40 bg-gray-900/80">
      <table className={`${fontClass} font-mono w-full border-collapse`}>
        <thead>
          <tr className="border-b border-green-700/30">
            {columns.map(col => (
              <th key={col} className={`${cellPad} text-left text-green-400/70 font-semibold whitespace-nowrap`}>
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className={i % 2 !== 0 ? 'bg-white/5' : ''}>
              {columns.map(col => (
                <td key={col} className={`${cellPad} text-gray-300 ${maxCellWidth} truncate whitespace-nowrap`}>
                  {String(row[col] ?? '')}
                </td>
              ))}
            </tr>
          ))}
          {remaining > 0 && (
            <tr>
              <td colSpan={columns.length} className={`${cellPad} text-gray-500 italic text-center`}>
                + {remaining} more rows
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
};
