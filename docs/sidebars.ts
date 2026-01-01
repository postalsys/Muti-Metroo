import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

/**
 * Sidebar configuration for Muti Metroo documentation.
 * Organized by user journey: Getting Started -> Concepts -> Features -> Reference -> Advanced
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    'download',
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
      items: [
        'getting-started/overview',
        'getting-started/installation',
        'getting-started/quick-start',
        'getting-started/interactive-setup',
        'getting-started/first-mesh',
      ],
    },
    {
      type: 'category',
      label: 'Core Concepts',
      items: [
        'concepts/architecture',
        'concepts/agent-roles',
        'concepts/transports',
        'concepts/routing',
        'concepts/streams',
      ],
    },
    {
      type: 'category',
      label: 'Configuration',
      items: [
        'configuration/overview',
        'configuration/agent',
        'configuration/listeners',
        'configuration/peers',
        'configuration/socks5',
        'configuration/exit',
        'configuration/tls-certificates',
        'configuration/environment-variables',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/socks5-proxy',
        'features/exit-routing',
        'features/file-transfer',
        'features/rpc',
        'features/web-dashboard',
        'features/metrics-monitoring',
      ],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: [
        'deployment/scenarios',
        'deployment/docker',
        'deployment/kubernetes',
        'deployment/system-service',
        'deployment/high-availability',
      ],
    },
    {
      type: 'category',
      label: 'Security',
      items: [
        'security/overview',
        'security/e2e-encryption',
        'security/tls-mtls',
        'security/authentication',
        'security/access-control',
        'security/best-practices',
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
        'api/debugging',
      ],
    },
    {
      type: 'category',
      label: 'Protocol Reference',
      items: [
        'protocol/overview',
        'protocol/frames',
        'protocol/routing-algorithm',
        'protocol/limits',
      ],
    },
    {
      type: 'category',
      label: 'Development',
      items: [
        'development/testing',
        'development/docker-dev',
      ],
    },
    {
      type: 'category',
      label: 'Troubleshooting',
      items: [
        'troubleshooting/common-issues',
        'troubleshooting/connectivity',
        'troubleshooting/performance',
        'troubleshooting/faq',
      ],
    },
  ],
};

export default sidebars;
