import React from 'react';
import { Link } from 'react-router-dom';
import { cn } from '@/lib/utils';

/**
 * StatCard — a compact stat display card with icon.
 *
 * Usage:
 * ```tsx
 * <StatCard title="Total Agents" value={42} icon={Computer} to="/agents" />
 * ```
 *
 * Props:
 *   title    — label beneath the value
 *   value    — the number (or string) to display
 *   icon     — lucide icon component
 *   to       — optional link target (wraps in react-router Link)
 *   color    — icon+background color classes (default text-gray-600 bg-gray-100)
 *   children — optional; if provided, replaces the value/icon row entirely
 */
interface StatCardProps {
  title: string;
  value?: string | number;
  icon?: React.ComponentType<{ className?: string }>;
  to?: string;
  color?: string;
  className?: string;
  children?: React.ReactNode;
}

const StatCard: React.FC<StatCardProps> = ({
  title,
  value,
  icon: Icon,
  to,
  color = 'text-gray-600 bg-gray-100',
  className,
  children,
}) => {
  const inner = (
    <div
      className={cn(
        'bg-white p-4 rounded-lg border border-gray-200 shadow-sm',
        to && 'hover:shadow-md transition-shadow cursor-pointer',
        className
      )}
    >
      {children ? (
        children
      ) : (
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-gray-600">{title}</p>
            <p className="text-2xl font-bold text-gray-900">{value}</p>
          </div>
          {Icon && (
            <div className={cn('p-2 rounded-lg', color)}>
              <Icon className="h-6 w-6" />
            </div>
          )}
        </div>
      )}
    </div>
  );

  if (to) {
    return <Link to={to}>{inner}</Link>;
  }

  return inner;
};

export default StatCard;

/**
 * StatCardGroup — two stat cards side-by-side with a visual divider.
 *
 * Used in Updates.tsx for Approved/Pending and Critical/High combined cards.
 *
 * Usage:
 * ```tsx
 * <StatCardGroup
 *   left={{ title: 'Approved', value: 5, icon: CheckCircle, color: '...' }}
 *   right={{ title: 'Pending', value: 12, icon: Clock, color: '...' }}
 * />
 * ```
 */
interface StatCardSideProps {
  title: string;
  value: string | number;
  icon?: React.ComponentType<{ className?: string }>;
  color?: string;
}

interface StatCardGroupProps {
  left: StatCardSideProps;
  right: StatCardSideProps;
  className?: string;
}

export const StatCardGroup: React.FC<StatCardGroupProps> = ({
  left,
  right,
  className,
}) => (
  <div className={cn('bg-white p-4 rounded-lg border border-gray-200 shadow-sm', className)}>
    <div className="flex items-center justify-between divide-x divide-gray-200">
      <div className="flex-1 pr-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs font-medium text-gray-600">{left.title}</p>
            <p className={cn('text-xl font-bold', left.color ? left.color.split(' ')[0] : 'text-gray-900')}>
              {left.value}
            </p>
          </div>
          {left.icon && (
            <left.icon className={cn('h-6 w-6', left.color ? left.color.split(' ')[0] : 'text-gray-400')} />
          )}
        </div>
      </div>
      <div className="flex-1 pl-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs font-medium text-gray-600">{right.title}</p>
            <p className={cn('text-xl font-bold', right.color ? right.color.split(' ')[0] : 'text-gray-900')}>
              {right.value}
            </p>
          </div>
          {right.icon && (
            <right.icon className={cn('h-6 w-6', right.color ? right.color.split(' ')[0] : 'text-gray-400')} />
          )}
        </div>
      </div>
    </div>
  </div>
);
