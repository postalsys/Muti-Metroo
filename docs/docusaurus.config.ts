import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Muti Metroo',
  tagline: 'Userspace mesh networking agent with multi-hop routing',
  favicon: 'img/favicon.ico',

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Enable Mermaid diagrams
  markdown: {
    mermaid: true,
  },
  themes: ['@docusaurus/theme-mermaid'],

  // Set the production url of your site here
  url: 'https://muti-metroo.postalsys.com',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'Coinstash', // Usually your GitHub org/user name.
  projectName: 'muti-metroo', // Usually your repo name.

  onBrokenLinks: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          routeBasePath: '/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Project social card
    image: 'img/logo.png',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Muti Metroo',
      logo: {
        alt: 'Muti Metroo Logo',
        src: 'img/logo.png',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Documentation',
        },
        {
          type: 'search',
          position: 'right',
        },
        {
          href: 'https://git.aiateibad.ee/andris/Muti-Metroo-v4',
          label: 'Source',
          position: 'right',
        },
      ],
    },
    // Uncomment and configure Algolia search when ready
    // algolia: {
    //   appId: 'YOUR_APP_ID',
    //   apiKey: 'YOUR_SEARCH_API_KEY',
    //   indexName: 'muti-metroo',
    //   contextualSearch: true,
    // },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Getting Started',
              to: '/getting-started/overview',
            },
            {
              label: 'Configuration',
              to: '/configuration/overview',
            },
            {
              label: 'CLI Reference',
              to: '/cli/overview',
            },
            {
              label: 'HTTP API',
              to: '/api/overview',
            },
          ],
        },
        {
          title: 'Features',
          items: [
            {
              label: 'SOCKS5 Proxy',
              to: '/features/socks5-proxy',
            },
            {
              label: 'Exit Routing',
              to: '/features/exit-routing',
            },
            {
              label: 'File Transfer',
              to: '/features/file-transfer',
            },
            {
              label: 'RPC',
              to: '/features/rpc',
            },
          ],
        },
        {
          title: 'Resources',
          items: [
            {
              label: 'Source Code',
              href: 'https://git.aiateibad.ee/andris/Muti-Metroo-v4',
            },
            {
              label: 'Protocol Details',
              to: '/protocol/overview',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Muti Metroo Project. All rights reserved.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'yaml', 'json', 'go'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
