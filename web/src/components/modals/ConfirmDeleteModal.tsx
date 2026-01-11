import React, { useEffect } from 'react';
import { Trash2, Check, X } from 'lucide-react';
import { Want } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { useConfirmationDialogKeyboard } from '@/hooks/useConfirmationDialogKeyboard';
import { BaseModal } from './BaseModal';
import { classNames } from '@/utils/helpers';

interface ConfirmDeleteModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  want: Want | null;
  loading?: boolean;
  childrenCount?: number;
  title?: string;
  message?: string;
}

export const ConfirmDeleteModal: React.FC<ConfirmDeleteModalProps> = ({
  isOpen,
  onClose,
  onConfirm,
  want,
  loading = false,
  childrenCount = 0
}) => {
  // Keyboard shortcuts: Y (Confirm), N (Cancel), Esc (handled by BaseModal)
  useEffect(() => {
    if (!isOpen || loading) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (['INPUT', 'TEXTAREA'].includes(target.tagName) || target.isContentEditable) {
        return;
      }

      if (e.key.toLowerCase() === 'y') {
        e.preventDefault();
        onConfirm();
      } else if (e.key.toLowerCase() === 'n') {
        e.preventDefault();
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, loading, onConfirm, onClose]);

  // Enterprise shortcut hook (Enter key)
  useConfirmationDialogKeyboard({
    isVisible: isOpen,
    onConfirm,
    onCancel: onClose,
    loading,
    enabled: isOpen && !loading
  });

  if (!want) return null;

  const wantName = want.metadata?.name || want.metadata?.id || want.id || 'Unnamed Want';

  const footer = (
    <div className="flex justify-end gap-3">
      <button
        onClick={onClose}
        disabled={loading}
        className={classNames(
          'flex items-center justify-center w-14 h-14 aspect-square rounded-xl border border-gray-200 shadow-sm',
          'bg-white text-gray-500 hover:bg-gray-50 hover:text-red-600 transition-all duration-200',
          'disabled:opacity-50 disabled:cursor-not-allowed'
        )}
        title="Cancel (N or Esc)"
      >
        <X className="h-7 w-7" />
      </button>
      <button
        onClick={onConfirm}
        disabled={loading}
        className={classNames(
          'flex items-center justify-center w-14 h-14 aspect-square rounded-xl shadow-md',
          'bg-red-600 text-white hover:bg-red-700 transition-all duration-200',
          'disabled:opacity-50 disabled:cursor-not-allowed'
        )}
        title="Delete (Y)"
      >
        {loading ? (
          <LoadingSpinner size="md" color="white" />
        ) : (
          <Check className="h-7 w-7" />
        )}
      </button>
    </div>
  );

  return (
    <BaseModal
      isOpen={isOpen}
      onClose={onClose}
      title="Delete Want"
      footer={footer}
      size="md"
    >
      <div className="space-y-6">
        <div className="flex items-center gap-4">
          <div className="flex-shrink-0 w-12 h-12 rounded-full bg-red-100 flex items-center justify-center">
            <Trash2 className="h-6 w-6 text-red-600" />
          </div>
          <div>
            <h4 className="text-xl font-bold text-gray-900">Are you sure?</h4>
            <p className="text-gray-500">
              This action cannot be undone.
            </p>
          </div>
        </div>

        <div className="bg-gray-50 rounded-xl p-4 space-y-2 border border-gray-100">
          <div className="flex justify-between">
            <span className="text-sm font-medium text-gray-500">Want Name</span>
            <span className="text-sm font-bold text-gray-900">{wantName}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm font-medium text-gray-500">ID</span>
            <span className="text-sm font-mono text-gray-700">{want.metadata?.id || want.id || 'N/A'}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-sm font-medium text-gray-500">Status</span>
            <span className="text-sm px-2 py-0.5 rounded bg-white border border-gray-200 font-semibold text-gray-700">
              {want.status}
            </span>
          </div>
        </div>

        {want.status === 'reaching' && (
          <div className="bg-amber-50 border-l-4 border-amber-400 p-4 rounded-r-xl">
            <div className="flex">
              <div className="flex-shrink-0">
                <svg className="h-5 w-5 text-amber-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="ml-3">
                <p className="text-sm text-amber-800 font-medium">
                  Currently Running
                </p>
                <p className="text-sm text-amber-700 mt-1">
                  This want is active. Deleting it will stop the execution immediately.
                </p>
              </div>
            </div>
          </div>
        )}

        {childrenCount > 0 && (
          <div className="bg-red-50 border-l-4 border-red-400 p-4 rounded-r-xl">
            <div className="flex">
              <div className="flex-shrink-0">
                <svg className="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M18 10a8 8 0 11-18 0 8 8 0 0118 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                </svg>
              </div>
              <div className="ml-3">
                <p className="text-sm text-red-800 font-medium">
                  Dependency Warning
                </p>
                <p className="text-sm text-red-700 mt-1">
                  This want has {childrenCount} child want{childrenCount > 1 ? 's' : ''}.
                  Deleting the parent will recursively delete all its descendants.
                </p>
              </div>
            </div>
          </div>
        )}
      </div>
    </BaseModal>
  );
};
