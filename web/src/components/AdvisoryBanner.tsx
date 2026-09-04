import React from 'react';
import { AlertTriangle } from 'lucide-react';
import { useAdvisoryHealth } from '@/hooks/useAdvisoryHealth';

// AdvisoryBanner makes the fail-open advisory feed loud instead of silent. When
// OSV/repology is unreachable the breaker fails open (patches still flow, manual
// approval still works) but the auto-confirm gate is fail-closed — so auto-approval
// is quietly suspended. This is the operator's heads-up that it happened.
const AdvisoryBanner: React.FC = () => {
  const { data } = useAdvisoryHealth();

  if (!data || !data.degraded) return null;

  const feedDown = data.osv.state !== 'closed';
  const deferred = data.deferred_packages;

  const headline = feedDown ? 'Advisory feed offline' : 'Advisory checks catching up';
  const detail = feedDown
    ? 'OSV unreachable — auto-approval is suspended. Manual approval still works.'
    : 'OSV recovered — re-vetting deferred packages.';

  let pkgNote = '';
  if (deferred > 0) {
    pkgNote = ` ${deferred} package${deferred === 1 ? '' : 's'} awaiting recheck.`;
  } else if (deferred < 0) {
    pkgNote = ' (deferred count unavailable).';
  }

  return (
    <div className="bg-amber-50 border-b border-amber-200 px-4 sm:px-6 lg:px-8 py-2">
      <div className="flex items-center gap-2 text-sm text-amber-900">
        <AlertTriangle className="w-4 h-4 flex-shrink-0 text-amber-600" />
        <span className="font-medium">{headline}</span>
        <span className="text-amber-800">
          {detail}
          {pkgNote}
        </span>
      </div>
    </div>
  );
};

export default AdvisoryBanner;
