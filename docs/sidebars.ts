import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

/**
 * Sidebar configuration for Muti Metroo documentation.
 * Organized by user journey: Learn -> Get -> Decide -> Setup -> Understand -> Configure -> Use -> Deploy -> Secure -> Reference -> Fix
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    'download',
    {
      type: 'category',
      label: 'Comparisons',
      items: [
        'comparisons/vs-ligolo-ng',
        'comparisons/vs-ssh-jump',
        'comparisons/vs-chisel',
        'comparisons/vs-gost',
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
        'configuration/icmp',
        'configuration/http',
        'configuration/shell',
        'configuration/file-transfer',
        'configuration/routing',
        'configuration/management',
        'configuration/tls-certificates',
        'configuration/environment-variables',
      ],
    },
    {
      type: 'category',
      label: 'Usage Guides',
      items: [
        'features/socks5-proxy',
        'features/udp-relay',
        'features/icmp-relay',
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
        'deployment/embedded-config',
        'deployment/docker',
        'deployment/system-service',
        'deployment/pm2',
        'deployment/dll-mode',
        'deployment/reverse-proxy',
        'deployment/high-availability',
        'mutiauk',
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
        'cli/status',
        'cli/peers',
        'cli/routes',
        'cli/probe',
        'cli/ping',
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
        'api/icmp',
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
