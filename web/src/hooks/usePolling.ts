import { useEffect, useRef } from 'react';

interface UsePollingOptions {
  interval: number;
  enabled?: boolean;
  immediate?: boolean;
}

export const usePolling = (
  callback: () => void | Promise<void>,
  options: UsePollingOptions
) => {
  const { interval, enabled = true, immediate = false } = options;
  const callbackRef = useRef(callback);
  const intervalRef = useRef<ReturnType<typeof setInterval>>();

  // Update callback ref when callback changes
  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!enabled) {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = undefined;
      }
      return;
    }

    const executeCallback = async () => {
      try {
        await callbackRef.current();
      } catch (error) {
        console.error('Polling callback error:', error);
      }
    };

    // Execute immediately if requested
    if (immediate) {
      executeCallback();
    }

    // Set up interval
    intervalRef.current = setInterval(executeCallback, interval);

    // Cleanup
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = undefined;
      }
    };
  }, [interval, enabled, immediate]);

  // Manual trigger
  const trigger = () => {
    callbackRef.current();
  };

  return { trigger };
};