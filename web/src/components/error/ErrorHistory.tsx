import React, { useEffect, useState } from 'react';
import {
  AlertTriangle,
  XCircle,
  AlertCircle,
  Clock,
  CheckCircle,
  Eye,
  Trash2,
  MessageSquare,
  Save,
  X,
  RefreshCw
} from 'lucide-react';
import { ErrorHistoryEntry } from '@/types/api';
import { useErrorHistoryStore } from '@/stores/errorHistoryStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { formatDate, classNames } from '@/utils/helpers';

interface ErrorHistoryProps {
  className?: string;
}

export const ErrorHistory: React.FC<ErrorHistoryProps> = ({ className = '' }) => {
  const {
    errors,
    selectedError,
    loading,
    error,
    fetchErrorHistory,
    selectError,
    markAsResolved,
    deleteErrorEntry,
    addNotes,
    clearError
  } = useErrorHistoryStore();

  const [editingNotes, setEditingNotes] = useState<string | null>(null);
  const [notesValue, setNotesValue] = useState('');

  useEffect(() => {
    fetchErrorHistory();
  }, [fetchErrorHistory]);

  const getErrorIcon = (errorEntry: ErrorHistoryEntry) => {
    if (errorEntry.resolved) return CheckCircle;
    if (errorEntry.type === 'validation') return AlertTriangle;
    if (errorEntry.status >= 500) return XCircle;
    return AlertCircle;
  };

  const getErrorColor = (errorEntry: ErrorHistoryEntry) => {
    if (errorEntry.resolved) return 'text-green-600 bg-green-50 border-green-200';
    if (errorEntry.type === 'validation') return 'text-yellow-600 bg-yellow-50 border-yellow-200';
    if (errorEntry.status >= 500) return 'text-red-600 bg-red-50 border-red-200';
    return 'text-orange-600 bg-orange-50 border-orange-200';
  };

  const getStatusColor = (errorEntry: ErrorHistoryEntry) => {
    if (errorEntry.resolved) return 'bg-green-100 text-green-800';
    if (errorEntry.status >= 500) return 'bg-red-100 text-red-800';
    if (errorEntry.status >= 400) return 'bg-yellow-100 text-yellow-800';
    return 'bg-gray-100 text-gray-800';
  };

  const handleMarkResolved = async (id: string) => {
    try {
      await markAsResolved(id);
    } catch (error) {
      console.error('Failed to mark error as resolved:', error);
    }
  };

  const handleDeleteError = async (id: string) => {
    if (window.confirm('Are you sure you want to delete this error entry?')) {
      try {
        await deleteErrorEntry(id);
        if (selectedError?.id === id) {
          selectError(null);
        }
      } catch (error) {
        console.error('Failed to delete error:', error);
      }
    }
  };

  const handleSaveNotes = async (id: string) => {
    try {
      await addNotes(id, notesValue);
      setEditingNotes(null);
      setNotesValue('');
    } catch (error) {
      console.error('Failed to save notes:', error);
    }
  };

  const startEditingNotes = (errorEntry: ErrorHistoryEntry) => {
    setEditingNotes(errorEntry.id);
    setNotesValue(errorEntry.notes || '');
  };

  const cancelEditingNotes = () => {
    setEditingNotes(null);
    setNotesValue('');
  };

  const handleRefresh = () => {
    fetchErrorHistory();
  };

  if (loading && errors.length === 0) {
    return (
      <div className={`flex items-center justify-center h-64 ${className}`}>
        <LoadingSpinner />
      </div>
    );
  }

  return (
    <div className={`space-y-6 w-full min-w-0 overflow-hidden ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex-1 min-w-0">
          <h2 className="text-2xl font-bold text-gray-900">Error History</h2>
          <p className="text-gray-600 mt-1">
            View and manage API errors that have occurred
          </p>
        </div>
        <button
          onClick={handleRefresh}
          disabled={loading}
          className="flex items-center px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50 disabled:opacity-50 flex-shrink-0"
        >
          <RefreshCw className={classNames('h-4 w-4 mr-2', loading && 'animate-spin')} />
          Refresh
        </button>
      </div>

      {/* Error message */}
      {error && (
        <div className="p-4 bg-red-50 border border-red-200 rounded-md">
          <div className="flex">
            <XCircle className="h-5 w-5 text-red-400" />
            <div className="ml-3">
              <h3 className="text-sm font-medium text-red-800">Error</h3>
              <p className="text-sm text-red-700 mt-1">{error}</p>
              <button
                onClick={clearError}
                className="text-sm text-red-600 hover:text-red-500 underline mt-2"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      )}


      {/* Error list */}
      {errors.length === 0 ? (
        <div className="text-center py-12">
          <CheckCircle className="h-12 w-12 text-gray-400 mx-auto mb-4" />
          <h3 className="text-lg font-medium text-gray-900 mb-2">No errors found</h3>
          <p className="text-gray-600">
            Great! There are no API errors in the history.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {[...errors].reverse().map((errorEntry) => {
            const Icon = getErrorIcon(errorEntry);
            const isEditing = editingNotes === errorEntry.id;

            return (
              <div
                key={errorEntry.id}
                className={classNames(
                  'border rounded-lg p-4 transition-all duration-200',
                  getErrorColor(errorEntry),
                  selectedError?.id === errorEntry.id && 'ring-2 ring-blue-500'
                )}
              >
                {/* Error header */}
                <div className="flex items-start justify-between">
                  <div className="flex items-start space-x-3">
                    <Icon className="h-5 w-5 mt-0.5 flex-shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center space-x-2">
                        <h4 className="text-sm font-medium truncate">
                          {errorEntry.method} {errorEntry.endpoint}
                        </h4>
                        <span className={classNames(
                          'px-2 py-1 text-xs font-medium rounded-full',
                          getStatusColor(errorEntry)
                        )}>
                          {errorEntry.status}
                        </span>
                        {errorEntry.resolved && (
                          <span className="px-2 py-1 text-xs font-medium bg-green-100 text-green-800 rounded-full">
                            Resolved
                          </span>
                        )}
                      </div>
                      <p className="text-sm mt-1">{errorEntry.message}</p>
                      {errorEntry.details && (
                        <p className="text-xs text-gray-600 mt-1">{errorEntry.details}</p>
                      )}
                      <div className="flex items-center text-xs text-gray-500 mt-2">
                        <Clock className="h-3 w-3 mr-1" />
                        {formatDate(errorEntry.timestamp)}
                        {errorEntry.type && (
                          <>
                            <span className="mx-2">•</span>
                            <span className="capitalize">{errorEntry.type}</span>
                          </>
                        )}
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center space-x-2 ml-4">
                    <button
                      onClick={() => selectError(selectedError?.id === errorEntry.id ? null : errorEntry)}
                      className="p-1 text-gray-400 hover:text-gray-600 rounded"
                      title="View details"
                    >
                      <Eye className="h-4 w-4" />
                    </button>
                    {!errorEntry.resolved && (
                      <button
                        onClick={() => handleMarkResolved(errorEntry.id)}
                        className="p-1 text-green-400 hover:text-green-600 rounded"
                        title="Mark as resolved"
                      >
                        <CheckCircle className="h-4 w-4" />
                      </button>
                    )}
                    <button
                      onClick={() => startEditingNotes(errorEntry)}
                      className="p-1 text-blue-400 hover:text-blue-600 rounded"
                      title="Add/edit notes"
                    >
                      <MessageSquare className="h-4 w-4" />
                    </button>
                    <button
                      onClick={() => handleDeleteError(errorEntry.id)}
                      className="p-1 text-red-400 hover:text-red-600 rounded"
                      title="Delete error"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                </div>

                {/* Notes section */}
                {(errorEntry.notes || isEditing) && (
                  <div className="mt-4 pt-4 border-t border-gray-200">
                    <div className="flex items-center justify-between mb-2">
                      <h5 className="text-sm font-medium text-gray-700">Notes</h5>
                    </div>
                    {isEditing ? (
                      <div className="space-y-2">
                        <textarea
                          value={notesValue}
                          onChange={(e) => setNotesValue(e.target.value)}
                          className="w-full p-2 text-sm border border-gray-300 rounded-md focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                          rows={3}
                          placeholder="Add notes about this error..."
                        />
                        <div className="flex space-x-2">
                          <button
                            onClick={() => handleSaveNotes(errorEntry.id)}
                            className="flex items-center px-3 py-1 text-sm bg-blue-600 text-white rounded-md hover:bg-blue-700"
                          >
                            <Save className="h-3 w-3 mr-1" />
                            Save
                          </button>
                          <button
                            onClick={cancelEditingNotes}
                            className="flex items-center px-3 py-1 text-sm bg-gray-600 text-white rounded-md hover:bg-gray-700"
                          >
                            <X className="h-3 w-3 mr-1" />
                            Cancel
                          </button>
                        </div>
                      </div>
                    ) : (
                      <p className="text-sm text-gray-600">{errorEntry.notes}</p>
                    )}
                  </div>
                )}

                {/* Detailed view */}
                {selectedError?.id === errorEntry.id && (
                  <div className="mt-4 pt-4 border-t border-gray-200 min-w-0">
                    <h5 className="text-sm font-medium text-gray-700 mb-3">Error Details</h5>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm min-w-0">
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700">ID:</span>
                        <span className="ml-2 text-gray-600 font-mono">{errorEntry.id}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700">Status:</span>
                        <span className="ml-2 text-gray-600">{errorEntry.status}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700">Method:</span>
                        <span className="ml-2 text-gray-600 font-mono">{errorEntry.method}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700">Endpoint:</span>
                        <span className="ml-2 text-gray-600 font-mono">{errorEntry.endpoint}</span>
                      </div>
                      {errorEntry.code && (
                        <div className="min-w-0">
                          <span className="font-medium text-gray-700">Code:</span>
                          <span className="ml-2 text-gray-600 font-mono">{errorEntry.code}</span>
                        </div>
                      )}
                      {errorEntry.userAgent && (
                        <div className="md:col-span-2 min-w-0">
                          <span className="font-medium text-gray-700">User Agent:</span>
                          <span className="ml-2 text-gray-600 text-xs">{errorEntry.userAgent}</span>
                        </div>
                      )}
                    </div>

                    {errorEntry.requestData && (
                      <div className="mt-4 min-w-0">
                        <span className="font-medium text-gray-700">Request Data:</span>
                        <pre className="mt-1 p-2 bg-gray-100 rounded text-xs overflow-x-auto min-w-0">
                          {JSON.stringify(errorEntry.requestData, null, 2)}
                        </pre>
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};