import { useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ErrorBoundary } from '@/components/common/ErrorBoundary';
import { Dashboard } from '@/pages/Dashboard';
import { LogsPage } from '@/pages/ErrorHistoryPage';
import { AgentsPage } from '@/pages/AgentsPage';
import RecipePage from '@/pages/RecipePage';
import WantTypePage from '@/pages/WantTypePage';
import { useConfigStore } from '@/stores/configStore';

function App() {
  const fetchConfig = useConfigStore(state => state.fetchConfig);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  return (
    <ErrorBoundary>
      <Router>
        <div className="App">
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/agents" element={<AgentsPage />} />
            <Route path="/recipes" element={<RecipePage />} />
            <Route path="/want-types" element={<WantTypePage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>
        </div>
      </Router>
    </ErrorBoundary>
  );
}

export default App;