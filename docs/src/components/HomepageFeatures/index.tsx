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
    title: 'End-to-End Encryption',
    image: '/img/mole-inspecting.png',
    imageAlt: 'Mole inspecting with magnifying glass',
    link: '/security/e2e-encryption',
    description: (
      <>
        X25519 key exchange with ChaCha20-Poly1305 encryption. Transit nodes
        relay traffic they cannot decrypt - zero trust by design.
      </>
    ),
  },
  {
    title: 'Multi-Hop Mesh Routing',
    image: '/img/mole-plumbing.png',
    imageAlt: 'Mole connecting pipes',
    link: '/concepts/routing',
    description: (
      <>
        Automatic route propagation across arbitrary topologies. Traffic flows
        through chains, trees, or full mesh with CIDR and domain-based routing.
      </>
    ),
  },
  {
    title: 'Flexible Transports',
    image: '/img/mole-wiring.png',
    imageAlt: 'Mole connecting wires',
    link: '/concepts/transports',
    description: (
      <>
        QUIC for performance, HTTP/2 for blending with HTTPS, or WebSocket for
        traversing corporate proxies and restrictive firewalls.
      </>
    ),
  },
  {
    title: 'No Root Required',
    image: '/img/mole-drilling.png',
    imageAlt: 'Mole drilling through layers',
    link: '/concepts/architecture',
    description: (
      <>
        Runs entirely in userspace as a single binary. No kernel modules,
        no elevated privileges. Deploy on containers, VMs, or bare metal.
      </>
    ),
  },
  {
    title: 'SOCKS5 and TUN Interface',
    image: '/img/mole-escalator.png',
    imageAlt: 'Mole on escalator',
    link: '/features/socks5-proxy',
    description: (
      <>
        SOCKS5 proxy for application-level integration, or TUN interface via{' '}
        <Link to="/mutiauk">Mutiauk</Link> for transparent routing. Route any
        traffic through the mesh without per-app configuration.
      </>
    ),
  },
  {
    title: 'Remote Operations',
    image: '/img/mole-presenting.png',
    imageAlt: 'Mole presenting',
    link: '/features/shell',
    description: (
      <>
        Execute commands and transfer files across the mesh. Interactive shell
        with command whitelisting and authenticated file upload/download.
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
