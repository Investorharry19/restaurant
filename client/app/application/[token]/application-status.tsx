"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ApiError, apiRequest } from "../../../lib/api/client";
import {
  decodePublicApplicationStatus,
  type PublicApplicationStatus,
} from "../../../lib/api/applications";
import styles from "../../onboarding/onboarding.module.css";

function getStatusError(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) {
    return "This application link is unavailable. Check that you have the full private link.";
  }

  if (error instanceof ApiError && error.status === 429) {
    return "Too many status checks. Wait a moment, then try again.";
  }

  return "We couldn't load your application right now. Check your connection and try again.";
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "long" }).format(
    new Date(value),
  );
}

export function ApplicationStatusPage({ token }: { token: string }) {
  const [application, setApplication] = useState<{
    token: string;
    value: PublicApplicationStatus;
  } | null>(null);
  const [requestState, setRequestState] = useState<{
    token: string;
    isLoading: boolean;
    error: string | null;
  }>({ token, isLoading: true, error: null });
  const [refreshVersion, setRefreshVersion] = useState(0);
  const currentApplication =
    application?.token === token ? application.value : null;
  const isLoading = requestState.token !== token || requestState.isLoading;
  const error = requestState.token === token ? requestState.error : null;

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    async function loadStatus() {
      try {
        const status = await apiRequest(
          `/api/public/applications/${encodeURIComponent(token)}`,
          decodePublicApplicationStatus,
          { signal: controller.signal },
        );
        if (active) {
          setApplication({ token, value: status });
          setRequestState({ token, isLoading: false, error: null });
        }
      } catch (requestError: unknown) {
        if (active) {
          setRequestState({
            token,
            isLoading: false,
            error: getStatusError(requestError),
          });
        }
      }
    }

    void loadStatus();

    return () => {
      active = false;
      controller.abort();
    };
  }, [refreshVersion, token]);

  return (
    <main className={styles.page} aria-busy={isLoading}>
      <div className={styles.statusShell}>
        <header className={styles.header}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
          <Link className={styles.headerLink} href="/onboarding">
            New application
          </Link>
        </header>

        {isLoading && !currentApplication ? (
          <section
            aria-label="Loading application status"
            className={styles.loadingState}
          >
            <p role="status">Loading your application</p>
            <div className={styles.skeletonLine} />
            <div className={styles.skeletonPanel} />
          </section>
        ) : currentApplication ? (
          <>
            <header className={styles.statusHeader}>
              <h1>{currentApplication.businessName}</h1>
              <p>Application sent {formatDate(currentApplication.createdAt)}</p>
            </header>

            {error && (
              <p className={styles.statusError} role="status">
                {error} Your last status is shown below.
              </p>
            )}

            <section aria-live="polite" className={styles.statusPanel}>
              <span className={styles.statusLabel}>
                {currentApplication.status === "PENDING"
                  ? "In review"
                  : currentApplication.status === "APPROVED"
                    ? "Approved"
                    : "Not approved"}
              </span>

              {currentApplication.status === "PENDING" ? (
                <>
                  <h2>Your application is being reviewed.</h2>
                  <p>
                    Keep this private link and refresh this page later to check
                    for a decision.
                  </p>
                </>
              ) : currentApplication.status === "APPROVED" ? (
                <>
                  <h2>Your restaurant page is ready.</h2>
                  <p>
                    Your guest entry page is live. Open your workspace to share
                    its QR code and manage restaurant operations.
                  </p>
                </>
              ) : (
                <>
                  <h2>This application was not approved.</h2>
                  <p>
                    <strong>Reason for rejection: </strong>
                    {currentApplication.reviewNote ??
                      "There is no additional note for this decision."}
                  </p>
                </>
              )}

              {currentApplication.reviewedAt && (
                <p>
                  Decision made {formatDate(currentApplication.reviewedAt)}.
                </p>
              )}

              <div className={styles.statusActions}>
                {currentApplication.status === "APPROVED" &&
                  currentApplication.restaurantSlug && (
                    <>
                      <Link
                        href={`/workspace/${encodeURIComponent(currentApplication.restaurantSlug)}`}
                      >
                        Open restaurant workspace
                      </Link>
                      <Link
                        href={`/r/${encodeURIComponent(currentApplication.restaurantSlug)}`}
                      >
                        Open guest page
                      </Link>
                    </>
                  )}
                {currentApplication.status === "REJECTED" && (
                  <Link
                    href={`/onboarding?resubmit=${encodeURIComponent(token)}`}
                  >
                    Edit and resubmit
                  </Link>
                )}
                <button
                  className={styles.refreshButton}
                  disabled={isLoading}
                  onClick={() => {
                    setRequestState({ token, isLoading: true, error: null });
                    setRefreshVersion((version) => version + 1);
                  }}
                  type="button"
                >
                  {isLoading ? "Refreshing…" : "Refresh status"}
                </button>
              </div>
            </section>

            <p className={styles.privacyNote}>
              This link is private. Anyone who has it can view this application
              status.
            </p>
          </>
        ) : (
          <section className={styles.statusPanel} role="alert">
            <h2>Application status unavailable</h2>
            <p>
              {error ??
                "We couldn't find this application. Check that you have the full private link."}
            </p>
            <div className={styles.statusActions}>
              <button
                className={styles.refreshButton}
                disabled={isLoading}
                onClick={() => {
                  setRequestState({ token, isLoading: true, error: null });
                  setRefreshVersion((version) => version + 1);
                }}
                type="button"
              >
                {isLoading ? "Trying again…" : "Try again"}
              </button>
              <Link href="/onboarding">Start a new application</Link>
            </div>
          </section>
        )}
      </div>
    </main>
  );
}
