import React, { useState } from 'react';
import { AlertTriangle, Activity, CheckCircle, XCircle } from 'lucide-react';
import { Layout } from '@/components/layout/Layout';
import { Header } from '@/components/layout/Header';
import { RightSidebar } from '@/components/layout/RightSidebar';
import { ErrorHistory } from '@/components/error/ErrorHistory';
import { LogHistory } from '@/components/logs/LogHistory';
import { useLogStore } from '@/stores/logStore';
import { useErrorHistoryStore } from '@/stores/errorHistoryStore';
import { classNames } from '@/utils/helpers';

type TabType = 'errors' | 'logs';

export const LogsPage: React.FC = () => {
  const [sidebarMinimized, setSidebarMinimized] = useState(true);
  const [activeTab, setActiveTab] = useState<TabType>('logs');
  const [showSummary, setShowSummary] = useState(false);

  const { logs } = useLogStore();
  const { errors } = useErrorHistoryStore();

  // Calculate statistics
  const logSuccessCount = logs.filter(l => l.status === 'success').length;
  const logErrorCount = logs.filter(l => l.status === 'error').length;
  const errorResolvedCount = errors.filter(e => e.resolved).length;

  return (
    <Layout
      sidebarMinimized={sidebarMinimized}
      onSidebarMinimizedChange={setSidebarMinimized}
    >
      {/* Header */}
      <Header
        title="Logs"
        onCreateWant={() => {}}
        sidebarMinimized={sidebarMinimized}
        showSummary={showSummary}
        onSummaryToggle={() => setShowSummary(!showSummary)}
        hideCreateButton={true}
      />

      {/* Main content area */}
      <main className="flex-1 flex overflow-hidden bg-gray-50 dark:bg-gray-950 lg:mr-[480px] mr-0 relative">
        <div className="flex-1 overflow-y-auto">
          <div className="p-6 pb-24">
            {/* Tab Navigation */}
            <div className="border-b border-gray-200 dark:border-gray-800">
              <nav className="-mb-px flex space-x-8">
                <button
                  onClick={() => setActiveTab('logs')}
                  className={classNames(
                    'group inline-flex items-center py-4 px-1 border-b-2 font-medium text-sm',
                    activeTab === 'logs'
                      ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-700'
                  )}
                >
                  <Activity className={classNames(
                    'mr-2 h-5 w-5',
                    activeTab === 'logs' ? 'text-blue-500 dark:text-blue-400' : 'text-gray-400 dark:text-gray-500 group-hover:text-gray-500 dark:group-hover:text-gray-400'
                  )} />
                  API Logs
                </button>
                <button
                  onClick={() => setActiveTab('errors')}
                  className={classNames(
                    'group inline-flex items-center py-4 px-1 border-b-2 font-medium text-sm',
                    activeTab === 'errors'
                      ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                      : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:border-gray-300 dark:hover:border-gray-700'
                  )}
                >
                  <AlertTriangle className={classNames(
                    'mr-2 h-5 w-5',
                    activeTab === 'errors' ? 'text-blue-500 dark:text-blue-400' : 'text-gray-400 dark:text-gray-500 group-hover:text-gray-500 dark:group-hover:text-gray-400'
                  )} />
                  Errors
                </button>
              </nav>
            </div>

            {/* Tab Content */}
            <div className="mt-6">
              {activeTab === 'errors' && <ErrorHistory />}
              {activeTab === 'logs' && <LogHistory />}
            </div>
          </div>
        </div>
      </main>

      {/* Right Sidebar for Summary */}
      <RightSidebar
        isOpen={showSummary}
        onClose={() => setShowSummary(false)}
        title="Summary"
      >
        <div className="space-y-6">
          <div>
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
              {activeTab === 'logs' ? 'API Logs' : 'Errors'} Statistics
            </h3>
            <div className="space-y-4">
              {activeTab === 'logs' ? (
                <>
                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <Activity className="h-8 w-8 text-gray-400 dark:text-gray-500" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Total Logs</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{logs.length}</p>
                      </div>
                    </div>
                  </div>

                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <CheckCircle className="h-8 w-8 text-green-400" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Success</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{logSuccessCount}</p>
                      </div>
                    </div>
                  </div>

                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <XCircle className="h-8 w-8 text-red-400" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Errors</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{logErrorCount}</p>
                      </div>
                    </div>
                  </div>
                </>
              ) : (
                <>
                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <AlertTriangle className="h-8 w-8 text-gray-400 dark:text-gray-500" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Total Errors</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{errors.length}</p>
                      </div>
                    </div>
                  </div>

                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <CheckCircle className="h-8 w-8 text-green-400" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Resolved</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{errorResolvedCount}</p>
                      </div>
                    </div>
                  </div>

                  <div className="bg-white dark:bg-gray-800 p-4 rounded-lg border border-gray-200 dark:border-gray-700">
                    <div className="flex items-center">
                      <div className="flex-shrink-0">
                        <XCircle className="h-8 w-8 text-red-400" />
                      </div>
                      <div className="ml-3">
                        <p className="text-sm font-medium text-gray-500 dark:text-gray-400">Unresolved</p>
                        <p className="text-2xl font-semibold text-gray-900 dark:text-white">{errors.length - errorResolvedCount}</p>
                      </div>
                    </div>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>
      </RightSidebar>
    </Layout>
  );
};