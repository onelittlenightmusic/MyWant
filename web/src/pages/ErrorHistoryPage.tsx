import React, { useState } from 'react';
import { Menu } from 'lucide-react';
import { Header } from '@/components/layout/Header';
import { Sidebar } from '@/components/layout/Sidebar';
import { ErrorHistory } from '@/components/error/ErrorHistory';
import { classNames } from '@/utils/helpers';

export const ErrorHistoryPage: React.FC = () => {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [sidebarMinimized, setSidebarMinimized] = useState(false); // Start expanded, auto-collapse on mouse leave

  return (
    <div className="min-h-screen bg-gray-50 flex">
      {/* Mobile sidebar toggle */}
      <div className="lg:hidden fixed top-4 left-4 z-40">
        <button
          onClick={() => setSidebarOpen(true)}
          className="p-2 rounded-md bg-white shadow-md border border-gray-200 text-gray-600 hover:text-gray-900"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Sidebar */}
      <Sidebar
        isOpen={sidebarOpen}
        isMinimized={sidebarMinimized}
        onClose={() => setSidebarOpen(false)}
        onMinimizeToggle={() => setSidebarMinimized(!sidebarMinimized)}
      />

      {/* Main content */}
      <div className={classNames(
        "flex-1 flex flex-col relative transition-all duration-300 ease-in-out",
        sidebarMinimized ? "lg:ml-20" : "lg:ml-64"
      )}>
        {/* Header */}
        <Header onCreateWant={() => {}} />

        {/* Main content area */}
        <main className="flex-1 p-6">
          <ErrorHistory />
        </main>
      </div>
    </div>
  );
};