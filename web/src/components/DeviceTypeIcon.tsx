import React from 'react';
import { Server, Monitor, Laptop, Smartphone, Tablet, Box, Container, Computer } from 'lucide-react';
import { cn } from '@/lib/utils';

// DeviceTypeIcon — consistent form-factor rendering (WEB-001).
// effective_device_type drives everything; unknown/absent falls back to the
// generic Computer icon so pre-migration agents render unchanged.

const DEVICE_ICONS: Record<string, React.ComponentType<{ className?: string }>> = {
  server: Server,
  desktop: Monitor,
  laptop: Laptop,
  phone: Smartphone,
  tablet: Tablet,
  vm: Box,
  container: Container,
};

const DEVICE_BADGE_CLASSES: Record<string, string> = {
  server: 'bg-gray-100 text-gray-700',
  desktop: 'bg-slate-100 text-slate-700',
  laptop: 'bg-sky-100 text-sky-700',
  phone: 'bg-emerald-100 text-emerald-700',
  tablet: 'bg-violet-100 text-violet-700',
  vm: 'bg-amber-100 text-amber-700',
  container: 'bg-cyan-100 text-cyan-700',
};

export const deviceTypeLabel = (type?: string): string => {
  if (!type) return 'Unknown';
  if (type === 'vm') return 'VM';
  return type.charAt(0).toUpperCase() + type.slice(1);
};

export const DeviceTypeIcon: React.FC<{ type?: string; className?: string }> = ({ type, className }) => {
  const Icon = (type && DEVICE_ICONS[type]) || Computer;
  return <Icon className={className || 'h-4 w-4'} />;
};

export const DeviceTypeBadge: React.FC<{ type?: string; overridden?: boolean; className?: string }> = ({
  type,
  overridden,
  className,
}) => {
  if (!type || !DEVICE_ICONS[type]) return null;
  return (
    <span
      className={cn(
        'inline-flex items-center text-xs px-1.5 py-0.5 rounded-full w-fit',
        DEVICE_BADGE_CLASSES[type],
        className
      )}
      title={overridden ? 'Device type set by operator' : 'Device type auto-detected'}
    >
      <DeviceTypeIcon type={type} className="h-3 w-3 mr-1" />
      {deviceTypeLabel(type)}
      {overridden && <span className="ml-1 opacity-60">*</span>}
    </span>
  );
};
