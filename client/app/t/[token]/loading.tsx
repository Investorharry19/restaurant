import styles from "./ticket-status.module.css";

export default function Loading() {
  return (
    <main className={styles.page} aria-busy="true">
      <div className={styles.shell}>
        <section aria-label="Loading ticket" className={styles.loadingState}>
          <p role="status">Loading your ticket</p>
          <div className={styles.skeletonTitle} />
          <div className={styles.skeletonPanel} />
          <div className={styles.skeletonText} />
        </section>
      </div>
    </main>
  );
}
