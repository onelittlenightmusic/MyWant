import { useEffect } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ErrorBoundary } from '@/components/common/ErrorBoundary';
import { Dashboard } from '@/pages/Dashboard';
import { LogsPage } from '@/pages/ErrorHistoryPage';
import { AgentsPage } from '@/pages/AgentsPage';
import RecipePage from '@/pages/RecipePage';
import WantTypePage from '@/pages/WantTypePage';
import { AchievementsPage } from '@/pages/AchievementsPage';
import { useConfigStore } from '@/stores/configStore';
import { Layout } from '@/components/layout/Layout';

async function loadExternalPlugins() {
  try {
    const res = await fetch('/api/v1/plugins')
    if (!res.ok) return
    const urls: string[] = await res.json()
    await Promise.allSettled(urls.map(url => import(/* @vite-ignore */ url)))
  } catch {
    // External plugins unavailable — continue without them
  }
}

function App() {
  const fetchConfig = useConfigStore(state => state.fetchConfig);

  useEffect(() => {
    fetchConfig();
    loadExternalPlugins();
  }, [fetchConfig]);

  return (
    <ErrorBoundary>
      <Router>
        <div className="App">
          <Layout>
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<Dashboard />} />
              <Route path="/agents" element={<AgentsPage />} />
              <Route path="/recipes" element={<RecipePage />} />
              <Route path="/want-types" element={<WantTypePage />} />
              <Route path="/logs" element={<LogsPage />} />
              <Route path="/achievements" element={<AchievementsPage />} />
              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </Layout>
        </div>
      </Router>
    </ErrorBoundary>
  );
}

export default App;