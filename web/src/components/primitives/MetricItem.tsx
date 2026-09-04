import React from 'react';
import { cn } from '@/lib/utils';

/**
 * MetricItem — a single system-info metric cell.
 *
 * Renders a label, value, optional icon, optional sub-text, and optional
 * progress bar. Designed to be an independent grid or flex item.
 *
 * Usage:
 *   <MetricItem label="CPU" value="Intel i7" icon={Cpu} sub="4 cores" />
 *   <MetricItem label="Disk" value="120 / 500 GB" icon={HardDrive}
 *     progress={{ used: 120, total: 500 }} />
 */
interface MetricItemProps {
  label: string;
  value: string | number;
  icon?: React.ComponentType<{ className?: string }>;
  sub?: string;
  progress?: { used: number; total: number };
  className?: string;
}

const MetricItem: React.FC<MetricItemProps> = ({
  label,
  value,
  icon: Icon,
  sub,
  progress,
  className,
}) => (
  <div className={cn('min-w-0', className)}>
    <p className="text-xs text-gray-500 flex items-center gap-1">
      {Icon && <Icon className="h-3 w-3" />}
      {label}
    </p>
    <p className="text-sm font-medium text-gray-900">{value}</p>
    {sub && <p className="text-xs text-gray-500">{sub}</p>}
    {progress && progress.total > 0 && (
      <>
        <div className="w-full bg-gray-200 rounded-full h-1.5 mt-1">
          <div
            className="bg-blue-600 h-1.5 rounded-full"
            style={{ width: `${Math.round((progress.used / progress.total) * 100)}%` }}
          />
        </div>
        <p className="text-xs text-gray-500">
          {Math.round((progress.used / progress.total) * 100)}% used
        </p>
      </>
    )}
  </div>
);

export default MetricItem;
