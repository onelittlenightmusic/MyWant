import React, { useEffect, useState } from 'react';
import {
  RefreshCw,
  CheckCircle,
  XCircle,
  Clock,
  Eye,
  Trash2,
  Activity
} from 'lucide-react';
import { LogEntry } from '@/types/api';
import { useLogStore } from '@/stores/logStore';
import { LoadingSpinner } from '@/components/common/LoadingSpinner';
import { formatDate, classNames } from '@/utils/helpers';

interface LogHistoryProps {
  className?: string;
}

export const LogHistory: React.FC<LogHistoryProps> = ({ className = '' }) => {
  const {
    logs,
    loading,
    error,
    fetchLogs,
    clearAllLogs,
    clearError
  } = useLogStore();

  const [expandedLog, setExpandedLog] = useState<string | null>(null);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const getMethodColor = (method: string) => {
    switch (method) {
      case 'POST': return 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300';
      case 'PUT': return 'bg-blue-100 dark:bg-blue-900/30 text-blue-800 dark:text-blue-300';
      case 'DELETE': return 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-300';
      default: return 'bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-100';
    }
  };

  const getStatusColor = (log: LogEntry) => {
    if (log.status === 'success') {
      return 'text-green-600 bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800';
    }
    return 'text-red-600 bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800';
  };

  const getStatusIcon = (log: LogEntry) => {
    return log.status === 'success' ? CheckCircle : XCircle;
  };

  const handleRefresh = () => {
    fetchLogs();
  };

  const handleClearLogs = async () => {
    if (window.confirm('Are you sure you want to clear all logs?')) {
      try {
        await clearAllLogs();
      } catch (error) {
        console.error('Failed to clear logs:', error);
      }
    }
  };

  const toggleExpand = (timestamp: string, index: number) => {
    const key = `${timestamp}-${index}`;
    setExpandedLog(expandedLog === key ? null : key);
  };

  if (loading && logs.length === 0) {
    return (
      <div className={`flex items-center justify-center h-64 ${className}`}>
        <LoadingSpinner />
      </div>
    );
  }

  const successCount = logs.filter(l => l.status === 'success').length;
  const errorCount = logs.filter(l => l.status === 'error').length;

  return (
    <div className={`space-y-6 w-full min-w-0 overflow-hidden ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between gap-4">
        <div className="flex-1 min-w-0">
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">API Logs</h2>
          <p className="text-gray-600 dark:text-gray-300 mt-1">
            View API operations (POST, PUT, DELETE)
          </p>
        </div>
        <div className="flex space-x-2 flex-shrink-0">
          <button
            onClick={handleRefresh}
            disabled={loading}
            className="flex items-center px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-800 disabled:opacity-50"
          >
            <RefreshCw className={classNames('h-4 w-4 mr-2', loading && 'animate-spin')} />
            Refresh
          </button>
          <button
            onClick={handleClearLogs}
            disabled={loading || logs.length === 0}
            className="flex items-center px-4 py-2 text-sm font-medium text-red-700 bg-white dark:bg-gray-800 border border-red-300 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50"
          >
            <Trash2 className="h-4 w-4 mr-2" />
            Clear All
          </button>
        </div>
      </div>

      {/* Error message */}
      {error && (
        <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
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


      {/* Log list */}
      {logs.length === 0 ? (
        <div className="text-center py-12">
          <Activity className="h-12 w-12 text-gray-400 dark:text-gray-500 mx-auto mb-4" />
          <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">No logs found</h3>
          <p className="text-gray-600 dark:text-gray-300">
            API operation logs will appear here.
          </p>
        </div>
      ) : (
        <div className="space-y-4 min-w-0">
          {[...logs].reverse().map((log, index) => {
            const Icon = getStatusIcon(log);
            const logKey = `${log.timestamp}-${index}`;
            const isExpanded = expandedLog === logKey;

            return (
              <div
                key={logKey}
                className={classNames(
                  'border rounded-lg p-4 transition-all duration-200',
                  getStatusColor(log)
                )}
              >
                {/* Log header */}
                <div className="flex items-start justify-between">
                  <div className="flex items-start space-x-3 flex-1 min-w-0">
                    <Icon className="h-5 w-5 mt-0.5 flex-shrink-0" />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center space-x-2 flex-wrap gap-2">
                        <span className={classNames(
                          'px-2 py-1 text-xs font-medium rounded-full whitespace-nowrap',
                          getMethodColor(log.method)
                        )}>
                          {log.method}
                        </span>
                        <h4 className="text-sm font-medium truncate">
                          {log.endpoint}
                        </h4>
                        <span className={classNames(
                          'px-2 py-1 text-xs font-medium rounded-full whitespace-nowrap',
                          log.status === 'success' ? 'bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300' : 'bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-300'
                        )}>
                          {log.statusCode}
                        </span>
                      </div>
                      {log.resource && (
                        <p className="text-sm mt-1 truncate">Resource: {log.resource}</p>
                      )}
                      {log.details && (
                        <p className="text-xs text-gray-600 dark:text-gray-300 mt-1 truncate">{log.details}</p>
                      )}
                      {log.errorMsg && (
                        <p className="text-sm text-red-700 mt-1 font-medium truncate">{log.errorMsg}</p>
                      )}
                      <div className="flex items-center text-xs text-gray-500 dark:text-gray-400 mt-2">
                        <Clock className="h-3 w-3 mr-1" />
                        {formatDate(log.timestamp)}
                      </div>
                    </div>
                  </div>

                  {/* Actions */}
                  <div className="flex items-center space-x-2 ml-4 flex-shrink-0">
                    <button
                      onClick={() => toggleExpand(log.timestamp, index)}
                      className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 rounded"
                      title="View details"
                    >
                      <Eye className="h-4 w-4" />
                    </button>
                  </div>
                </div>

                {/* Detailed view */}
                {isExpanded && (
                  <div className="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700 min-w-0">
                    <h5 className="text-sm font-medium text-gray-700 dark:text-gray-200 mb-3">Full Details</h5>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm min-w-0">
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Timestamp:</span>
                        <span className="ml-2 text-gray-600 dark:text-gray-300 font-mono text-xs break-all">{log.timestamp}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Method:</span>
                        <span className="ml-2 text-gray-600 dark:text-gray-300 font-mono">{log.method}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Endpoint:</span>
                        <span className="ml-2 text-gray-600 dark:text-gray-300 font-mono text-xs break-all">{log.endpoint}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Status Code:</span>
                        <span className="ml-2 text-gray-600 dark:text-gray-300">{log.statusCode}</span>
                      </div>
                      <div className="min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Status:</span>
                        <span className="ml-2 text-gray-600 dark:text-gray-300 capitalize">{log.status}</span>
                      </div>
                      {log.resource && (
                        <div className="min-w-0">
                          <span className="font-medium text-gray-700 dark:text-gray-200">Resource:</span>
                          <span className="ml-2 text-gray-600 dark:text-gray-300 break-all">{log.resource}</span>
                        </div>
                      )}
                    </div>
                    {log.details && (
                      <div className="mt-4 min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Details:</span>
                        <p className="mt-1 p-2 bg-gray-100 dark:bg-gray-700 rounded text-sm break-words">{log.details}</p>
                      </div>
                    )}
                    {log.errorMsg && (
                      <div className="mt-4 min-w-0">
                        <span className="font-medium text-gray-700 dark:text-gray-200">Error Message:</span>
                        <p className="mt-1 p-2 bg-red-100 dark:bg-red-900/20 rounded text-sm text-red-800 dark:text-red-300 break-words">{log.errorMsg}</p>
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
