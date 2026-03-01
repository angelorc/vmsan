export default defineNuxtConfig({
  extends: ['docus'],

  css: ['~/assets/css/main.css'],

  devtools: { enabled: true },

  site: {
    name: 'vmsan',
    url: 'https://vmsan.dev',
  },

  $development: {
    site: {
      url: 'http://localhost:3000',
    },
  },

  content: {
    experimental: {
      sqliteConnector: 'native',
    },
    build: {
      markdown: {
        highlight: {
          langs: ['bash', 'json', 'js', 'ts', 'shell', 'yaml', 'docker', 'diff'],
        },
      },
    },
  },

  mdc: {
    highlight: {
      shikiEngine: 'javascript',
    },
  },

  experimental: {
    asyncContext: true,
  },

  compatibilityDate: '2025-07-22',

  nitro: {
    prerender: {
      crawlLinks: true,
      failOnError: false,
      autoSubfolderIndex: false,
      routes: ['/'],
    },
  },

  llms: {
    domain: 'https://vmsan.dev',
    title: 'vmsan',
    description: 'Firecracker microVM sandbox toolkit. Spin up isolated microVMs in milliseconds with full lifecycle management, network isolation, Docker image support, and interactive shell access.',
    notes: [
      'vmsan is a CLI tool and TypeScript library for managing Firecracker microVMs.',
      'It provides VM lifecycle management, network isolation policies, file operations, and Docker image support.',
    ],
  },

  robots: {
    groups: [{
      userAgent: '*',
      allow: '/',
    }],
    sitemap: '/sitemap.xml',
  },

  icon: {
    clientBundle: {
      scan: true,
    },
    provider: 'iconify',
  },

  // WORKAROUND: unjs/unimport#273 + unjs/mlly#303
  // Now solved by pinning unimport to 5.6.0 in package.json overrides.
  // Keeping this commented out as a fallback in case the pin is removed.
  // vite: {
  //   plugins: [{
  //     name: 'fix-useResizable-options-import',
  //     enforce: 'post' as const,
  //     transform(code: string, _id: string) {
  //       // Strip the incorrectly injected `import { options }` from useResizable
  //       if (code.includes('useResizable') && /import\s*\{[^}]*\boptions\b[^}]*\}\s*from\s*["'][^"']*useResizable[^"']*["']/.test(code)) {
  //         return code.replace(
  //           /import\s*\{(\s*options\s*)\}\s*from\s*["'][^"']*useResizable[^"']*["'];?\s*/g,
  //           '',
  //         ).replace(
  //           /import\s*\{([^}]*),\s*options\s*\}\s*from\s*["']([^"']*useResizable[^"']*)"[;]?\s*/g,
  //           'import {$1} from "$2";',
  //         ).replace(
  //           /import\s*\{\s*options\s*,([^}]*)\}\s*from\s*["']([^"']*useResizable[^"']*)"[;]?\s*/g,
  //           'import {$1} from "$2";',
  //         )
  //       }
  //     },
  //   }],
  // },
})
