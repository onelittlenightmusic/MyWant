import React, { useState } from 'react';
import { BaseModal } from './BaseModal';
import { useConfigStore } from '@/stores/configStore';
import { Monitor, Sun, Moon, Layout as LayoutIcon, ArrowUp, ArrowDown, Power, RotateCcw, ShieldAlert } from 'lucide-react';
import { classNames } from '@/utils/helpers';
import { apiClient } from '@/api/client';

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({ isOpen, onClose }) => {
  const { config, updateConfig } = useConfigStore();
  const [isProcessing, setIsProcessing] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  if (!config) return null;

  const colorModes = [
    { id: 'light', label: 'Light', icon: Sun },
    { id: 'dark', label: 'Dark', icon: Moon },
    { id: 'system', label: 'System', icon: Monitor },
  ] as const;

  const headerPositions = [
    { id: 'top', label: 'Top', icon: ArrowUp },
    { id: 'bottom', label: 'Bottom', icon: ArrowDown },
  ] as const;

  const handleStopServer = async () => {
    if (!confirm('Are you sure you want to STOP the server? You will lose connection to this dashboard.')) return;
    
    setIsProcessing(true);
    try {
      const res = await apiClient.stopServer();
      setMessage(res.message);
    } catch (err) {
      setMessage('Failed to stop server');
    } finally {
      setIsProcessing(false);
    }
  };

  const handleRestartServer = async () => {
    if (!confirm('Are you sure you want to RESTART the server? The dashboard will briefly lose connection.')) return;

    setIsProcessing(true);
    try {
      const res = await apiClient.restartServer();
      setMessage(res.message);
      // Optional: reload page after a delay
      setTimeout(() => {
        window.location.reload();
      }, 5000);
    } catch (err) {
      setMessage('Failed to restart server');
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <BaseModal isOpen={isOpen} onClose={onClose} title="Settings" size="sm">
      <div className="space-y-8">
        {/* Color Mode */}
        <section>
          <h4 className="text-sm font-semibold text-gray-900 dark:text-white mb-4 flex items-center">
            <Sun className="w-4 h-4 mr-2" />
            Appearance
          </h4>
          <div className="grid grid-cols-3 gap-3">
            {colorModes.map((mode) => (
              <button
                key={mode.id}
                onClick={() => updateConfig({ color_mode: mode.id })}
                className={classNames(
                  "flex flex-col items-center justify-center p-3 rounded-xl border-2 transition-all",
                  config.color_mode === mode.id
                    ? "border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
                    : "border-gray-100 bg-white text-gray-600 hover:border-gray-200 dark:bg-gray-800 dark:border-gray-700 dark:text-gray-400"
                )}
              >
                <mode.icon className="w-6 h-6 mb-2" />
                <span className="text-xs font-medium">{mode.label}</span>
              </button>
            ))}
          </div>
        </section>

        {/* Header Position */}
        <section>
          <h4 className="text-sm font-semibold text-gray-900 dark:text-white mb-4 flex items-center">
            <LayoutIcon className="w-4 h-4 mr-2" />
            Layout (Mobile Friendly)
          </h4>
          <div className="grid grid-cols-2 gap-3">
            {headerPositions.map((pos) => (
              <button
                key={pos.id}
                onClick={() => updateConfig({ header_position: pos.id })}
                className={classNames(
                  "flex items-center justify-center p-4 rounded-xl border-2 transition-all",
                  config.header_position === pos.id
                    ? "border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300"
                    : "border-gray-100 bg-white text-gray-600 hover:border-gray-200 dark:bg-gray-800 dark:border-gray-700 dark:text-gray-400"
                )}
              >
                <pos.icon className="w-5 h-5 mr-3" />
                <span className="text-sm font-medium">{pos.label}</span>
              </button>
            ))}
          </div>
          <p className="mt-3 text-xs text-gray-500 dark:text-gray-400 italic">
            "Bottom" position is recommended for one-handed operation on mobile devices.
          </p>
        </section>

        {/* System Control */}
        <section className="pt-4 border-t border-gray-100 dark:border-gray-800">
          <h4 className="text-sm font-semibold text-gray-900 dark:text-white mb-4 flex items-center text-red-600 dark:text-red-400">
            <ShieldAlert className="w-4 h-4 mr-2" />
            System Control
          </h4>
          <div className="grid grid-cols-2 gap-3">
            <button
              onClick={handleRestartServer}
              disabled={isProcessing}
              className="flex items-center justify-center p-3 rounded-xl border-2 border-amber-100 bg-amber-50 text-amber-700 hover:bg-amber-100 transition-all dark:bg-amber-900/20 dark:border-amber-900/30 dark:text-amber-400 disabled:opacity-50"
            >
              <RotateCcw className="w-5 h-5 mr-2" />
              <span className="text-sm font-medium">Restart</span>
            </button>
            <button
              onClick={handleStopServer}
              disabled={isProcessing}
              className="flex items-center justify-center p-3 rounded-xl border-2 border-red-100 bg-red-50 text-red-700 hover:bg-red-100 transition-all dark:bg-red-900/20 dark:border-red-900/30 dark:text-red-400 disabled:opacity-50"
            >
              <Power className="w-5 h-5 mr-2" />
              <span className="text-sm font-medium">Stop</span>
            </button>
          </div>
          {message && (
            <p className="mt-3 text-xs text-center font-medium text-primary-600 dark:text-primary-400 animate-pulse">
              {message}
            </p>
          )}
        </section>
      </div>
    </BaseModal>
  );
};
