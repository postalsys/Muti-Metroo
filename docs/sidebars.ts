import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation

 The sidebars can be generated from the filesystem, or explicitly defined here.

 Create as many sidebars as you want.
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started',
        'installation',
        'quick-start',
        'interactive-setup',
      ],
    },
    'configuration',
    {
      type: 'category',
      label: 'Architecture',
      items: [
        'architecture/overview',
        'architecture/agent-roles',
        'architecture/data-flow',
        'architecture/packages',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/socks5',
        'features/exit-routing',
        'features/file-transfer',
        'features/rpc',
        'features/web-dashboard',
      ],
    },
    {
      type: 'category',
      label: 'CLI Reference',
      items: [
        'cli/overview',
        'cli/run',
        'cli/init',
        'cli/setup',
        'cli/cert',
        'cli/rpc',
        'cli/file-transfer',
        'cli/service',
      ],
    },
    {
      type: 'category',
      label: 'HTTP API',
      items: [
        'api/overview',
        'api/health',
        'api/metrics',
        'api/agents',
        'api/routes',
        'api/rpc',
        'api/file-transfer',
        'api/dashboard',
      ],
    },
    {
      type: 'category',
      label: 'Protocol',
      items: [
        'protocol/overview',
        'protocol/frames',
        'protocol/limits',
        'protocol/routing',
      ],
    },
    {
      type: 'category',
      label: 'Development',
      items: [
        'development/building',
        'development/testing',
        'development/docker',
      ],
    },
  ],
};

export default sidebars;
