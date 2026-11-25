import styles from "../../onboarding/onboarding.module.css";

export default function Loading() {
  return (
    <main className={styles.page} aria-busy="true">
      <div className={styles.statusShell}>
        <p className={styles.loadingState} role="status">
          Loading your application
        </p>
        <div className={styles.skeletonLine} />
        <div className={styles.skeletonPanel} />
      </div>
    </main>
  );
}
