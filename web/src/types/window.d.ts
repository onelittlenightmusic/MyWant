import type { WantCardPlugin } from '@/components/dashboard/WantCard/plugins/registry'

declare global {
  interface Window {
    React: typeof import('react')
    __mywant: {
      registerPlugin: (plugin: WantCardPlugin) => void
    }
  }
}
