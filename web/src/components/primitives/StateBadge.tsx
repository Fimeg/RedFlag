import React from 'react';
import { cn, getStatusColor, getSeverityColor } from '@/lib/utils';

// State badge primitives — the one way to render a lifecycle status or a
// severity as a badge. Colour mapping lives in primitives/statusColors.ts
// (packageStatusColor / packageSeverityColor); lib/utils re-exports them as
// getStatusColor / getSeverityColor for backwards compat.

interface BadgeProps {
  /** Extra classes merged onto the badge (e.g. 'text-[10px]', 'ml-auto'). */
  className?: string;
  /** Override the rendered text; defaults to the status/severity value. */
  label?: React.ReactNode;
}

export const StatusBadge: React.FC<BadgeProps & { status: string }> = ({ status, className, label }) => (
  <span className={cn('badge', getStatusColor(status), className)}>{label ?? status}</span>
);

export const SeverityBadge: React.FC<BadgeProps & { severity: string }> = ({ severity, className, label }) => (
  <span className={cn('badge', getSeverityColor(severity), className)}>{label ?? severity}</span>
);
