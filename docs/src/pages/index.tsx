import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <div className={styles.heroContent}>
          <div className={styles.heroText}>
            <Heading as="h1" className="hero__title">
              {siteConfig.title}
            </Heading>
            <p className="hero__subtitle">{siteConfig.tagline}</p>
            <div className={styles.buttons}>
              <Link
                className="button button--secondary button--lg"
                to="/getting-started/quick-start">
                Get Started
              </Link>
              <Link
                className="button button--outline button--secondary button--lg"
                style={{marginLeft: '1rem'}}
                to="/intro">
                Learn More
              </Link>
            </div>
          </div>
          <div className={styles.heroImage}>
            <img src="/img/mole-surfacing.png" alt="Muti Metroo Mole" />
          </div>
        </div>
      </div>
    </header>
  );
}

function UseCases(): ReactNode {
  return (
    <section className={styles.useCases}>
      <div className="container">
        <div className="row">
          <div className="col col--12">
            <Heading as="h2" className="text--center margin-bottom--lg">
              Use Cases
            </Heading>
          </div>
        </div>
        <div className="row">
          <div className="col col--4">
            <div className={styles.useCase}>
              <Heading as="h3">Corporate Network Access</Heading>
              <p>
                Access internal resources securely from anywhere. Create encrypted
                tunnels through corporate firewalls without VPN infrastructure.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className={styles.useCase}>
              <Heading as="h3">Multi-Site Connectivity</Heading>
              <p>
                Connect multiple data centers, offices, or cloud regions through
                a unified mesh network with automatic failover.
              </p>
            </div>
          </div>
          <div className="col col--4">
            <div className={styles.useCase}>
              <Heading as="h3">Firewall Traversal</Heading>
              <p>
                Traverse restrictive firewalls using WebSocket or HTTP/2 transports
                that work through corporate proxies.
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function QuickExample(): ReactNode {
  return (
    <section className={styles.example}>
      <div className="container">
        <div className="row">
          <div className="col col--12">
            <Heading as="h2" className="text--center margin-bottom--lg">
              Quick Example
            </Heading>
          </div>
        </div>
        <div className="row">
          <div className="col col--6">
            <Heading as="h4">1. Initialize and Run</Heading>
            <pre className={styles.codeBlock}>
{`# Initialize agent
./muti-metroo init -d ./data

# Run with config
./muti-metroo run -c config.yaml`}
            </pre>
          </div>
          <div className="col col--6">
            <Heading as="h4">2. Connect via SOCKS5</Heading>
            <pre className={styles.codeBlock}>
{`# Use curl through the mesh
curl -x socks5://localhost:1080 https://internal.example.com

# SSH through the mesh
ssh -o ProxyCommand='nc -x localhost:1080 %h %p' user@host`}
            </pre>
          </div>
        </div>
        <div className="row margin-top--lg">
          <div className="col col--12 text--center">
            <Link
              className="button button--primary button--lg"
              to="/getting-started/first-mesh">
              Build Your First Mesh
            </Link>
          </div>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title="Userspace Mesh Networking"
      description="Muti Metroo is a userspace mesh networking agent that creates virtual TCP tunnels across heterogeneous transport layers with multi-hop routing.">
      <HomepageHeader />
      <main>
        <HomepageFeatures />
        <UseCases />
        <QuickExample />
      </main>
    </Layout>
  );
}
