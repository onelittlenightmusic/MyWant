import { useEffect, useState } from 'react';

/**
 * Returns 'dark' or 'light' based on the current Tailwind color mode
 * (the `dark` class on <html>). Re-renders when the class changes.
 */
export function useColorMode(): 'dark' | 'light' {
  const [isDark, setIsDark] = useState<boolean>(() =>
    document.documentElement.classList.contains('dark'),
  );

  useEffect(() => {
    const root = document.documentElement;
    const observer = new MutationObserver(() => {
      setIsDark(root.classList.contains('dark'));
    });
    observer.observe(root, { attributes: true, attributeFilter: ['class'] });
    return () => observer.disconnect();
  }, []);

  return isDark ? 'dark' : 'light';
}
