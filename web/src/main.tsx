import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
import './styles/index.css'
import { registerWantCardPlugin } from './components/dashboard/WantCard/plugins/registry'

// Expose globals for dynamically loaded external plugins
window.React = React
window.__mywant = { registerPlugin: registerWantCardPlugin }

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)