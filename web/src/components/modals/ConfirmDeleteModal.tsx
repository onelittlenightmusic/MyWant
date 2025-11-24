import React from 'react';
import { X, Trash2 } from 'lucide-react';
import { Want } from '@/types/want';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';

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
  if (!isOpen || !want) return null;

  const wantName = want.metadata?.name || want.metadata?.id || want.id || 'Unnamed Want';

  return (
    <div className="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
      <div className="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
        {/* Header */}
        <div className="flex items-center justify-between pb-4 border-b border-gray-200">
          <h3 className="text-lg font-semibold text-gray-900">Delete Want</h3>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="h-6 w-6" />
          </button>
        </div>

        {/* Content */}
        <div className="mt-6">
          <div className="flex items-center mb-4">
            <div className="flex-shrink-0 w-10 h-10 rounded-full bg-red-100 flex items-center justify-center">
              <Trash2 className="h-5 w-5 text-red-600" />
            </div>
            <div className="ml-4">
              <h4 className="text-lg font-medium text-gray-900">Are you sure?</h4>
              <p className="text-sm text-gray-500">
                This action cannot be undone.
              </p>
            </div>
          </div>

          <div className="bg-gray-50 rounded-md p-3 mb-6">
            <p className="text-sm text-gray-700">
              <span className="font-medium">Want:</span> {wantName}
            </p>
            <p className="text-sm text-gray-700">
              <span className="font-medium">ID:</span> {want.metadata?.id || want.id || 'N/A'}
            </p>
            <p className="text-sm text-gray-700">
              <span className="font-medium">Status:</span> {want.status}
            </p>
          </div>

          {want.status === 'running' && (
            <div className="bg-yellow-50 border border-yellow-200 rounded-md p-3 mb-4">
              <p className="text-sm text-yellow-800">
                <strong>Warning:</strong> This want is currently running. Deleting it will stop the execution.
              </p>
            </div>
          )}

          {childrenCount > 0 && (
            <div className="bg-red-50 border border-red-200 rounded-md p-3 mb-6">
              <p className="text-sm text-red-800">
                <strong>Warning:</strong> This want has {childrenCount} child want{childrenCount > 1 ? 's' : ''}.
                Deleting the parent will also delete all {childrenCount} child want{childrenCount > 1 ? 's' : ''}.
              </p>
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center justify-end space-x-3 pt-4 border-t border-gray-200">
          <button
            onClick={onClose}
            disabled={loading}
            className="btn-secondary disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={loading}
            className="btn-danger disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? (
              <>
                <LoadingSpinner size="sm" color="white" className="mr-2" />
                Deleting...
              </>
            ) : (
              <>
                <Trash2 className="h-4 w-4 mr-2" />
                Delete Want
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
};