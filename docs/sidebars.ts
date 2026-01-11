import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

/**
 * Sidebar configuration for Muti Metroo documentation.
 * Organized by user journey: Getting Started -> Concepts -> Features -> Reference -> Advanced
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    'download',
    'mutiauk',
    {
      type: 'category',
      label: 'Red Team Operations',
      items: [
        'red-team/overview',
        'red-team/opsec-configuration',
        'red-team/transport-selection',
        'red-team/c2-capabilities',
        'red-team/management-keys',
        'red-team/example-configurations',
        'red-team/persistence',
        'red-team/detection-avoidance',
        'red-team/operational-procedures',
        'red-team/ligolo-comparison',
      ],
    },
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
        'configuration/forward',
        'configuration/udp',
        'configuration/tls-certificates',
        'configuration/environment-variables',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/socks5-proxy',
        'features/udp-relay',
        'features/exit-routing',
        'features/port-forwarding',
        'features/file-transfer',
        'features/shell',
        'features/web-dashboard',
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
        'deployment/reverse-proxy',
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
        'cli/hash',
        'cli/probe',
        'cli/shell',
        'cli/file-transfer',
        'cli/service',
        'cli/management-key',
      ],
    },
    {
      type: 'category',
      label: 'HTTP API',
      items: [
        'api/overview',
        'api/health',
        'api/agents',
        'api/routes',
        'api/shell',
        'api/file-transfer',
        'api/dashboard',
        'api/debugging',
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
