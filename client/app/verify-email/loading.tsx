import styles from "../account/owner-account.module.css";

export default function Loading() {
  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <section
          aria-labelledby="verify-loading-title"
          className={styles.accountPanel}
        >
          <h1 id="verify-loading-title">Verify your email.</h1>
          <p className={styles.status} role="status">
            Opening your verification link…
          </p>
        </section>
      </div>
    </main>
  );
}
