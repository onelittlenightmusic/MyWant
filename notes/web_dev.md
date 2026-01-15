# Vite and React Performance Optimization

## Overview

Vite has become the new standard for frontend development in 2025-2026, replacing Create React App (CRA) which has been deprecated. Vite's revolutionary approach uses Native ES Modules (ESM) to serve code on-demand during development, resulting in dev server startup times under 300ms, compared to Webpack's slower upfront bundling approach.

## Build Tool Configuration

### SWC for Maximum Performance

**Use SWC instead of Babel** for React transformation - Vite's Rust-based compiler offers significantly faster compilation and hot module reloads, providing the most impactful performance improvement for 2025-2026.

### Vite 6 Optimizations

- **Use Rolldown instead of Rollup and esbuild** for faster builds and a more aligned experience between dev and production environments
- When using `@vitejs/plugin-react`, avoid configuring Babel options so it skips transformation during build, allowing only esbuild to be used for faster compilation
- The `build.target` option allows targeting modern browsers only, reducing build time by excluding legacy support transformations

### Dev Server Performance

Using `--open` or `server.open` provides a performance boost as Vite will automatically warm up the entry point of your app or the provided URL to open.

## Code Splitting and Bundle Optimization

### Dynamic Imports

Implement code splitting with dynamic imports and `manualChunks` configuration to isolate third-party libraries and frequently used components, leading to better caching and reduced load times.

javascript
// vite.config.js
export default {
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom'],
          'ui-library': ['@mui/material']
        }
      }
    }
  }
}


### Bundle Analysis

Use `rollup-plugin-visualizer` for bundle analysis to identify large dependencies and optimize bundle composition effectively.

### Library Selection

Prefer ES modules (lodash-es, date-fns) over CommonJS libraries to enable better tree-shaking with Vite + Rollup.

## Asset Optimization

### SVG Handling

**Don't transform SVGs into UI framework components** (React, Vue, etc.) - import them as strings or URLs instead for better performance.

### CSS Optimization

- Enable CSS code splitting in Vite (`build.cssCodeSplit: true`)
- Purge unused CSS via PostCSS plugins for optimized styles
- Inline critical CSS for faster initial page loads

### Images and Fonts

- Optimize images before deployment
- Preload fonts to prevent layout shifts
- Use modern image formats (WebP, AVIF)

## React-Specific Optimizations

### Prevent Unnecessary Re-renders

Use React.memo for pure components, and useCallback/useMemo to avoid recreating functions and objects, preventing unnecessary re-renders even with fast build tools.

jsx
// Memoize expensive components
const ExpensiveComponent = React.memo(({ data }) => {
  return <div>{/* Complex rendering */}</div>;
});

// Memoize callbacks
const handleClick = useCallback(() => {
  // Handler logic
}, [dependencies]);

// Memoize computed values
const processedData = useMemo(() => {
  return expensiveOperation(data);
}, [data]);


## Testing

**Vitest is the recommended testing tool** for Vite projects - Jest-compatible but faster, designed specifically for Vite's architecture.

## Production Build Checklist

Ensure your production builds meet these performance benchmarks:

- ✅ Bundle size < 500KB (initial load)
- ✅ Lighthouse score > 90
- ✅ Code splitting implemented
- ✅ Images optimized
- ✅ Fonts preloaded
- ✅ Critical CSS inlined

## Quick Wins Summary

1. **Switch to SWC** - Most impactful single change
2. **Use Rolldown** (Vite 6+) - Better dev/prod alignment
3. **Enable `--open`** - Automatic warmup
4. **Avoid SVG transformations** - Better asset performance
5. **Choose ES modules** - Better tree-shaking
6. **Skip Babel in build** - Faster compilation

---

## Sources

- [Advanced Guide to Using Vite with React in 2025](https://codeparrot.ai/blogs/advanced-guide-to-using-vite-with-react-in-2025)
- [Vite vs Webpack for React Apps (2025 Senior Engineer Perspective)](https://blog.logrocket.com/vite-vs-webpack-react-apps-2025-senior-engineer/)
- [Vite Performance Guide](https://vite.dev/guide/performance)
- [React TypeScript Vite Production Setup](https://oneuptime.com/blog/post/2026-01-08-react-typescript-vite-production-setup/view)
- [Stop Waiting for Your React App to Load - The 2026 Guide to Vite](https://medium.com/@shubhspatil77/stop-waiting-for-your-react-app-to-load-the-2026-guide-to-vite-7e071923ab9f)
- [Performance Optimizations in React with Vite.js](https://elanchezhiyan-p.medium.com/performance-optimizations-in-react-with-vite-js-a4656f5e06fc)
- [React 2025: Building Modern Apps with Vite](https://www.joaovinezof.com/blog/react-2025-building-modern-apps-with-vite)
- [Optimizing Your React Vite Application - A Guide to Reducing Bundle Size](https://shaxadd.medium.com/optimizing-your-react-vite-application-a-guide-to-reducing-bundle-size-6b7e93891c96)
- [How to Optimize Vite App](https://dev.to/yogeshgalav7/how-to-optimize-vite-app-i89)
- [Optimize Vite Build Time - A Comprehensive Guide](https://dev.to/perisicnikola37/optimize-vite-build-time-a-comprehensive-guide-4c99)