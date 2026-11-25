"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ApiError, apiRequest } from "../../lib/api/client";
import { decodeOwnerAccountSession } from "../../lib/api/applications";
import styles from "../account/owner-account.module.css";

export function VerifyOwnerEmail({ token }: { token: string }) {
  const router = useRouter();
  const [errorMessage, setErrorMessage] = useState<string | null>(() =>
    token ? null : "This verification link is missing its token.",
  );

  useEffect(() => {
    if (!token) {
      return;
    }

    const controller = new AbortController();
    let active = true;
    void apiRequest("/api/public/accounts/verify", decodeOwnerAccountSession, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
      signal: controller.signal,
    })
      .then(() => {
        if (active) {
          router.replace("/onboarding");
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setErrorMessage(
            error instanceof ApiError &&
              error.code === "OWNER_VERIFICATION_INVALID"
              ? error.message
              : "We couldn't verify this email address. Request a new link and try again.",
          );
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [router, token]);

  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <header className={styles.header}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
        </header>
        <section aria-labelledby="verify-title" className={styles.accountPanel}>
          <h1 id="verify-title">Verify your email.</h1>
          {errorMessage ? (
            <>
              <p className={styles.errorMessage} role="alert">
                {errorMessage}
              </p>
              <p className={styles.intro}>
                You can request a fresh link from your owner account page.
              </p>
              <Link className={styles.primaryButton} href="/account">
                Go to owner account
              </Link>
            </>
          ) : (
            <p className={styles.status} role="status">
              Verifying your email and opening your business application…
            </p>
          )}
        </section>
      </div>
    </main>
  );
}
