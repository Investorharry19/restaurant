"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest } from "../../../lib/api/client";
import {
  decodeStaffInvitationDetails,
  decodeStaffSession,
  type StaffInvitationDetails,
} from "../../../lib/api/workspace";
import styles from "../../account/owner-account.module.css";

export function AcceptStaffInvite({ token }: { token: string }) {
  const router = useRouter();
  const [details, setDetails] = useState<StaffInvitationDetails | null>(null);
  const [isLoading, setIsLoading] = useState(() => Boolean(token));
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(
    token ? null : "This invitation link is missing its token.",
  );

  useEffect(() => {
    if (!token) {
      return;
    }

    const controller = new AbortController();
    let active = true;
    void apiRequest(
      `/api/public/staff-invitations/${encodeURIComponent(token)}`,
      decodeStaffInvitationDetails,
      { signal: controller.signal },
    )
      .then((invitation) => {
        if (active) {
          setDetails(invitation);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setErrorMessage(
            error instanceof ApiError
              ? error.message
              : "We couldn't load this invitation. Check your connection and try again.",
          );
        }
      })
      .finally(() => {
        if (active) {
          setIsLoading(false);
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [token]);

  async function accept(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting || !token) {
      return;
    }
    const formData = new FormData(event.currentTarget);
    setErrorMessage(null);
    setIsSubmitting(true);
    try {
      const session = await apiRequest(
        "/api/public/staff-invitations/accept",
        decodeStaffSession,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            token,
            name: String(formData.get("name") ?? "").trim(),
            password: String(formData.get("password") ?? ""),
          }),
        },
      );
      router.replace(
        `/workspace/${encodeURIComponent(session.restaurantSlug)}`,
      );
    } catch (error: unknown) {
      setErrorMessage(
        error instanceof ApiError
          ? error.message
          : "We couldn't accept this invitation. Check your connection and try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <header className={styles.header}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
        </header>
        <section aria-labelledby="invite-title" className={styles.accountPanel}>
          <h1 id="invite-title">Join the restaurant team.</h1>
          {isLoading ? (
            <p className={styles.status} role="status">
              Checking your invitation…
            </p>
          ) : errorMessage ? (
            <>
              <p className={styles.errorMessage} role="alert">
                {errorMessage}
              </p>
              <Link className={styles.primaryButton} href="/account">
                Go to owner account
              </Link>
            </>
          ) : details ? (
            <>
              <p className={styles.intro}>
                You’re invited to {details.restaurantName} as a{" "}
                {details.role.toLowerCase()}.
              </p>
              <p className={styles.status}>
                Invitation for {details.email}. Use your existing account
                password, or choose a password if this is your first account.
              </p>
              <form
                className={styles.form}
                onSubmit={(event) => void accept(event)}
              >
                <label className={styles.field}>
                  Your name
                  <input
                    autoComplete="name"
                    maxLength={120}
                    name="name"
                    required
                  />
                </label>
                <label className={styles.field}>
                  Password
                  <input
                    autoComplete="new-password"
                    minLength={12}
                    name="password"
                    required
                    type="password"
                  />
                  <small>Use at least 12 characters.</small>
                </label>
                {errorMessage && (
                  <p className={styles.errorMessage} role="alert">
                    {errorMessage}
                  </p>
                )}
                <button
                  className={styles.primaryButton}
                  disabled={isSubmitting}
                  type="submit"
                >
                  {isSubmitting ? "Joining…" : "Accept invitation"}
                </button>
              </form>
            </>
          ) : (
            <p className={styles.errorMessage} role="alert">
              This invitation is unavailable.
            </p>
          )}
        </section>
      </div>
    </main>
  );
}
