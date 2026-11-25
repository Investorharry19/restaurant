import Link from "next/link";
import styles from "./homepage.module.css";

const queuePaths = [
  {
    title: "Table waitlist",
    description:
      "Join a table waitlist and check your ticket status and queue progress while you wait.",
    mapDescription: "Join the table queue.",
  },
  {
    title: "Pickup order",
    description: "Follow a pickup ticket and check when your order is ready.",
    mapDescription: "Check when it is ready.",
  },
];

const journeySteps = [
  {
    title: "Scan",
    description: "Open the restaurant’s guest page from its QR code.",
  },
  {
    title: "Choose",
    description: "Select table wait or pickup from the options shown.",
  },
  {
    title: "Check",
    description:
      "Use your ticket to check its status and, for table waits, queue progress.",
  },
];

export default function Home() {
  return (
    <div className={styles.page} id="top">
      <a className={styles.skipLink} href="#main-content">
        Skip to content
      </a>

      <div className={styles.shell}>
        <header className={styles.header}>
          <a className={styles.wordmark} href="#top">
            Restaurant queues
          </a>
          <nav aria-label="Main navigation">
            <a className={styles.navLink} href="#how-it-works">
              How it works
              <ArrowIcon />
            </a>
            <Link className={styles.navLink} href="/account">
              For restaurants
            </Link>
          </nav>
        </header>
      </div>

      <main id="main-content">
        <div className={styles.shell}>
          <section aria-labelledby="hero-title" className={styles.hero}>
            <div className={styles.heroCopy}>
              <h1 id="hero-title">
                Your place in line,
                <span> on your phone.</span>
              </h1>
              <p className={styles.heroDescription}>
                Scan a restaurant’s QR code to join a table waitlist or follow a
                pickup order. Check your ticket’s status as you wait.
              </p>
              <div className={styles.heroActions}>
                <a className={styles.primaryLink} href="#how-it-works">
                  See how it works
                  <ArrowIcon />
                </a>
                <span className={styles.accountNote}>
                  No full account needed
                </span>
              </div>
            </div>

            <RouteDiagram />
          </section>

          <section
            aria-labelledby="paths-title"
            className={styles.pathsSection}
          >
            <div className={styles.sectionIntro}>
              <h2 id="paths-title">One entry point. Two ways to wait.</h2>
              <p>
                Each restaurant’s guest page shows the queue options available
                there.
              </p>
            </div>
            <div className={styles.pathList}>
              {queuePaths.map((path) => (
                <article className={styles.pathItem} key={path.title}>
                  <span className={styles.pathNode} aria-hidden="true" />
                  <div>
                    <h3>{path.title}</h3>
                    <p>{path.description}</p>
                  </div>
                </article>
              ))}
            </div>
          </section>
        </div>

        <section
          aria-labelledby="how-title"
          className={styles.howSection}
          id="how-it-works"
        >
          <div className={styles.shell}>
            <div className={styles.howIntro}>
              <h2 id="how-title">Scan. Choose. Check.</h2>
              <p>
                Start from the restaurant’s QR entry, then keep your ticket
                handy on your phone.
              </p>
            </div>
            <ol className={styles.steps}>
              {journeySteps.map((step) => (
                <li className={styles.step} key={step.title}>
                  <h3>{step.title}</h3>
                  <p>{step.description}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>
      </main>

      <div className={styles.shell}>
        <footer className={styles.footer}>
          <p>Queue options and availability vary by restaurant.</p>
          <a href="#top">
            Back to top
            <ArrowIcon />
          </a>
        </footer>
      </div>
    </div>
  );
}

function RouteDiagram() {
  return (
    <figure aria-labelledby="route-caption" className={styles.routeFigure}>
      <figcaption className={styles.visuallyHidden} id="route-caption">
        Scan a restaurant QR code, choose a table waitlist or pickup order, and
        check your ticket on your phone.
      </figcaption>
      <div className={styles.routeMap}>
        <svg
          aria-hidden="true"
          className={styles.connectors}
          preserveAspectRatio="none"
          viewBox="0 0 640 392"
        >
          <path className={styles.connector} d="M320 52 V78" pathLength={1} />
          <path
            className={styles.connector}
            d="M320 78 C320 118 160 112 160 152"
            pathLength={1}
          />
          <path
            className={styles.connector}
            d="M320 78 C320 118 480 112 480 152"
            pathLength={1}
          />
          <path
            className={styles.connector}
            d="M160 246 C160 286 320 278 320 318"
            pathLength={1}
          />
          <path
            className={styles.connector}
            d="M480 246 C480 286 320 278 320 318"
            pathLength={1}
          />
        </svg>

        <div className={styles.entryNode}>
          <QrIcon />
          <span>
            <span className={styles.nodeLabel}>START HERE</span>
            <strong>Restaurant QR</strong>
          </span>
        </div>

        <ol className={styles.branchList}>
          {queuePaths.map((path) => (
            <li className={styles.branch} key={path.title}>
              <span className={styles.branchDot} aria-hidden="true" />
              <strong>{path.title}</strong>
              <span>{path.mapDescription}</span>
            </li>
          ))}
        </ol>

        <div className={styles.ticketNode}>
          <TicketIcon />
          <span>
            <span className={styles.nodeLabel}>YOUR NEXT STEP</span>
            <strong>Check your ticket</strong>
          </span>
          <span className={styles.phoneLabel}>ON YOUR PHONE</span>
        </div>
      </div>
    </figure>
  );
}

function ArrowIcon() {
  return (
    <svg
      aria-hidden="true"
      className={styles.arrowIcon}
      fill="none"
      viewBox="0 0 20 20"
    >
      <path d="M3.5 10h12m-5-5 5 5-5 5" />
    </svg>
  );
}

function QrIcon() {
  return (
    <svg
      aria-hidden="true"
      className={styles.queueIcon}
      fill="none"
      viewBox="0 0 32 32"
    >
      <path d="M5 12V5h7M20 5h7v7M27 20v7h-7M12 27H5v-7" />
      <path d="M11 11h3v3h-3zM18 11h3v3h-3zM11 18h3v3h-3zM18 19h3m-3 4h3m3-5v5" />
    </svg>
  );
}

function TicketIcon() {
  return (
    <svg
      aria-hidden="true"
      className={styles.queueIcon}
      fill="none"
      viewBox="0 0 32 32"
    >
      <path d="M6 9h20v5a3 3 0 0 0 0 6v5H6v-5a3 3 0 0 0 0-6V9Z" />
      <path d="M16 12v2m0 4v2" />
    </svg>
  );
}
