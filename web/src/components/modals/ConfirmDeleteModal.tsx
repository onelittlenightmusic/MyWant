import React from 'react';
import { Trash2 } from 'lucide-react';
import { Want } from '@/types/want';
import { ConfirmationBubble } from '@/components/notifications';

interface ConfirmDeleteModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  want: Want | null;
  loading?: boolean;
  childrenCount?: number;
  title?: string;
  message?: string;
  layout?: 'bottom-center' | 'inline-header' | 'dashboard-right';
}

export const ConfirmDeleteModal: React.FC<ConfirmDeleteModalProps> = ({
  isOpen,
  onClose,
  onConfirm,
  want,
  loading = false,
  childrenCount = 0,
  layout = 'dashboard-right' // Default to side context
}) => {
  if (!want) return null;

  const wantName = want.metadata?.name || want.metadata?.id || want.id || 'Unnamed Want';

  return (
    <ConfirmationBubble
      isVisible={isOpen}
      onDismiss={onClose}
      onConfirm={onConfirm}
      onCancel={onClose}
      loading={loading}
      title="Delete"
      layout={layout}
      message={null}
    >
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-red-600">
          <Trash2 className="h-4 w-4" />
          <span className="font-bold">Permanently delete?</span>
        </div>
        
        <div className="bg-gray-50 rounded-lg p-3 border border-gray-100 text-xs space-y-1">
          <div className="flex justify-between">
            <span className="text-gray-400">Target</span>
            <span className="font-bold text-gray-700 truncate max-w-[150px]">{wantName}</span>
          </div>
          {childrenCount > 0 && (
            <div className="pt-1 mt-1 border-t border-gray-200 text-red-500 font-bold">
              ⚠️ Includes {childrenCount} children
            </div>
          )}
        </div>
      </div>
    </ConfirmationBubble>
  );
};