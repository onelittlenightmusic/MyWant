import React from 'react';
import { classNames } from '@/utils/helpers';
import { ConfirmationBubble } from '@/components/notifications';
import { useConfigStore } from '@/stores/configStore';

interface HeaderOverlayProps {
  isVisible: boolean;
  children?: React.ReactNode;
  confirmationVisible?: boolean;
  confirmationTitle?: string;
  confirmationDanger?: boolean;
  onConfirmAction?: () => void;
  onCancelAction?: () => void;
  loading?: boolean;
}

export const HeaderOverlay: React.FC<HeaderOverlayProps> = ({
  isVisible,
  children,
  confirmationVisible = false,
  confirmationTitle = 'Confirm',
  confirmationDanger = false,
  onConfirmAction,
  onCancelAction,
  loading = false,
}) => {
  const config = useConfigStore(state => state.config);
  const isBottom = config?.header_position === 'bottom';

  if (!isVisible) return null;

  return (
    <div
      className={classNames('fixed left-0 right-0 z-50 overflow-hidden', isBottom ? 'bottom-0' : '')}
      style={{
        ...(isBottom ? {} : { top: 'env(safe-area-inset-top, 0px)' }),
        height: 'var(--header-height, 64px)',
        animation: 'quickActionsIn 150ms ease-out forwards',
      }}
    >
      {/* Backdrop: Dark in light mode, whitish in dark mode */}
      <div className="absolute inset-0 bg-black/80 dark:bg-white/90" />

      {/* Action bar content: Black text only in dark mode */}
      {children && (
        <div className="relative z-10 h-full dark:text-black">
          {children}
        </div>
      )}

      {/* Confirmation sub-overlay - covers entire header area */}
      <ConfirmationBubble
        isVisible={confirmationVisible}
        onConfirm={onConfirmAction || (() => {})}
        onCancel={onCancelAction || (() => {})}
        onDismiss={onCancelAction || (() => {})}
        title={confirmationTitle}
        layout="header-overlay"
        danger={confirmationDanger}
        loading={loading}
      />
    </div>
  );
};
