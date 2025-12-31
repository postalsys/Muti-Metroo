import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  image: string;
  imageAlt: string;
  description: ReactNode;
  link: string;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Multi-Transport Support',
    image: '/img/mole-wiring.png',
    imageAlt: 'Mole connecting wires',
    link: '/concepts/transports',
    description: (
      <>
        Choose the right transport for your environment: QUIC for performance,
        HTTP/2 for compatibility, or WebSocket for traversing restrictive firewalls.
      </>
    ),
  },
  {
    title: 'Userspace Operation',
    image: '/img/mole-drilling.png',
    imageAlt: 'Mole drilling through layers',
    link: '/concepts/architecture',
    description: (
      <>
        Runs entirely in userspace without kernel modules or root privileges.
        Deploy anywhere - containers, VMs, or bare metal.
      </>
    ),
  },
  {
    title: 'Mesh Networking',
    image: '/img/mole-plumbing.png',
    imageAlt: 'Mole connecting pipes',
    link: '/concepts/routing',
    description: (
      <>
        Automatic multi-hop routing with flood-based route propagation.
        Build arbitrary topologies: chains, trees, or full mesh.
      </>
    ),
  },
  {
    title: 'SOCKS5 Proxy',
    image: '/img/mole-escalator.png',
    imageAlt: 'Mole on escalator',
    link: '/features/socks5-proxy',
    description: (
      <>
        Standard SOCKS5 proxy interface for transparent application integration.
        Works with browsers, SSH, curl, and any SOCKS5-aware app.
      </>
    ),
  },
  {
    title: 'TLS/mTLS Security',
    image: '/img/mole-inspecting.png',
    imageAlt: 'Mole inspecting with magnifying glass',
    link: '/security/tls-mtls',
    description: (
      <>
        All connections secured with TLS 1.3 and perfect forward secrecy.
        Mutual TLS ensures only authorized agents can join.
      </>
    ),
  },
  {
    title: 'Production Ready',
    image: '/img/mole-presenting.png',
    imageAlt: 'Mole presenting',
    link: '/features/metrics-monitoring',
    description: (
      <>
        Built-in Prometheus metrics, health endpoints, web dashboard,
        and systemd/Windows service support.
      </>
    ),
  },
];

function Feature({title, image, imageAlt, description, link}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <Link to={link} className={styles.featureLink}>
        <div className={styles.featureCard}>
          <div className="text--center padding-vert--md">
            <img src={image} alt={imageAlt} className={styles.featureImage} />
          </div>
          <div className="text--center padding-horiz--md">
            <Heading as="h3">{title}</Heading>
            <p>{description}</p>
          </div>
        </div>
      </Link>
    </div>
  );
}

export default function HomepageFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
