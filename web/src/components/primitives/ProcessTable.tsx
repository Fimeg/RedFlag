import React from 'react';
import { Activity } from 'lucide-react';
import { cn } from '@/lib/utils';

/**
 * ProcessTable — top processes display for the System Information grid.
 *
 * Renders a compact table (Name, PID, CPU%, Mem%) when process data is
 * available, or a fallback message with the process count.
 *
 * Designed as an independent grid or flex item.
 */
interface ProcessInfo {
  name: string;
  pid: number;
  cpu: number | null;
  mem: number | null;
}

interface ProcessTableProps {
  processes?: ProcessInfo[];
  processCount?: number | string;
  onSeeMore?: () => void;
  className?: string;
}

const ProcessTable: React.FC<ProcessTableProps> = ({
  processes,
  processCount,
  onSeeMore,
  className,
}) => (
  <div className={cn('min-w-0', className)}>
    <div className="flex items-center justify-between mb-2">
      <p className="text-xs font-medium text-gray-900 flex items-center gap-1">
        <Activity className="h-3 w-3" /> Top Processes
      </p>
      {onSeeMore && (
        <button onClick={onSeeMore} className="text-[10px] text-blue-600 hover:text-blue-800">
          See More →
        </button>
      )}
    </div>
    {processes && processes.length > 0 ? (
      <table className="w-full text-xs">
        <thead>
          <tr className="text-gray-500 border-b border-gray-100">
            <th className="text-left py-1 font-medium">Name</th>
            <th className="text-right py-1 font-medium">PID</th>
            <th className="text-right py-1 font-medium">CPU%</th>
            <th className="text-right py-1 font-medium">Mem%</th>
          </tr>
        </thead>
        <tbody>
          {processes.slice(0, 5).map((proc, i) => (
            <tr key={proc.pid || i} className="border-b border-gray-50 last:border-0">
              <td className="py-1 text-gray-900 font-medium truncate max-w-[160px]">{proc.name}</td>
              <td className="py-1 text-right text-gray-600">{proc.pid}</td>
              <td className="py-1 text-right text-gray-600">
                {proc.cpu != null ? `${proc.cpu.toFixed(1)}%` : '—'}
              </td>
              <td className="py-1 text-right text-gray-600">
                {proc.mem != null ? `${proc.mem.toFixed(1)}%` : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    ) : (
      <p className="text-[10px] text-gray-400 italic">
        Process details not reported. {processCount != null && `Count: ${processCount}`}
      </p>
    )}
  </div>
);

export default ProcessTable;
