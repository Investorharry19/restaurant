"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest } from "../../lib/api/client";
import {
  decodeApprovedApplication,
  decodeBusinessApplications,
  decodeEmptyObject,
  decodePlatformAdminSession,
  type ApplicationListFilter,
  type BusinessApplication,
  type PlatformAdminSession,
} from "../../lib/api/applications";
import styles from "./platform-admin.module.css";

function getLoginError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 429) {
      return "Too many sign-in attempts. Wait a little and try again.";
    }
    if (error.code === "PLATFORM_AUTH_INVALID_CREDENTIALS") {
      return error.message;
    }
  }

  return "We couldn't sign you in. Check your connection and try again.";
}

export function PlatformAdminLogin() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    void apiRequest("/api/platform/auth/me", decodePlatformAdminSession, {
      signal: controller.signal,
    })
      .then(() => {
        if (active) {
          router.replace("/platform/applications");
        }
      })
      .catch((error: unknown) => {
        if (
          active &&
          (!(error instanceof ApiError) ||
            error.code !== "PLATFORM_AUTH_REQUIRED")
        ) {
          setErrorMessage(
            "We couldn't check your sign-in status. You can still try signing in.",
          );
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [router]);

  async function submitLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }

    const formData = new FormData(event.currentTarget);
    setErrorMessage(null);
    setIsSubmitting(true);
    try {
      await apiRequest("/api/platform/auth/login", decodePlatformAdminSession, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: String(formData.get("email") ?? "").trim(),
          password: String(formData.get("password") ?? ""),
        }),
      });
      router.replace("/platform/applications");
    } catch (error: unknown) {
      setErrorMessage(getLoginError(error));
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

        <section aria-labelledby="login-title" className={styles.loginSection}>
          <h1 id="login-title">Platform administrator</h1>
          <p>Sign in to review restaurant applications.</p>

          <form className={styles.loginForm} onSubmit={submitLogin}>
            <label className={styles.field}>
              <span>Email</span>
              <input
                autoComplete="username"
                maxLength={320}
                name="email"
                required
                type="email"
              />
            </label>
            <label className={styles.field}>
              <span>Password</span>
              <input
                autoComplete="current-password"
                maxLength={1024}
                name="password"
                required
                type="password"
              />
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
              {isSubmitting ? "Signing in…" : "Sign in"}
            </button>
          </form>

          <Link className={styles.homeLink} href="/">
            Back to the guest homepage
          </Link>
        </section>
      </div>
    </main>
  );
}

const applicationFilters: {
  value: ApplicationListFilter;
  label: string;
}[] = [
  { value: "PENDING", label: "Needs review" },
  { value: "ALL", label: "All applications" },
  { value: "APPROVED", label: "Approved" },
  { value: "REJECTED", label: "Not approved" },
];

function isApplicationListFilter(
  value: string,
): value is ApplicationListFilter {
  return applicationFilters.some((option) => option.value === value);
}

type ReviewNotice = {
  message: string;
  restaurantSlug?: string;
};

export function PlatformApplications() {
  const router = useRouter();
  const [admin, setAdmin] = useState<PlatformAdminSession | null>(null);
  const [applications, setApplications] = useState<BusinessApplication[]>([]);
  const [filter, setFilter] = useState<ApplicationListFilter>("PENDING");
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<ReviewNotice | null>(null);
  const [actionPendingID, setActionPendingID] = useState<string | null>(null);
  const [rejectingID, setRejectingID] = useState<string | null>(null);
  const [refreshVersion, setRefreshVersion] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    async function loadApplications() {
      setIsLoading(true);
      setLoadError(null);
      try {
        const identity = await apiRequest(
          "/api/platform/auth/me",
          decodePlatformAdminSession,
          { signal: controller.signal },
        );
        if (!active) {
          return;
        }
        setAdmin(identity);

        const query = new URLSearchParams({ status: filter });
        const rows = await apiRequest(
          `/api/platform/applications?${query.toString()}`,
          decodeBusinessApplications,
          { signal: controller.signal },
        );
        if (active) {
          setApplications(rows);
        }
      } catch (error: unknown) {
        if (!active) {
          return;
        }
        if (
          error instanceof ApiError &&
          error.code === "PLATFORM_AUTH_REQUIRED"
        ) {
          router.replace("/platform/login");
          return;
        }
        setLoadError(
          "We couldn't load applications. Check your connection and try again.",
        );
      } finally {
        if (active) {
          setIsLoading(false);
        }
      }
    }

    void loadApplications();

    return () => {
      active = false;
      controller.abort();
    };
  }, [filter, refreshVersion, router]);

  async function signOut() {
    setActionError(null);
    try {
      await apiRequest("/api/platform/auth/logout", decodeEmptyObject, {
        method: "POST",
      });
      router.replace("/platform/login");
    } catch {
      setActionError("We couldn't sign you out. Try again.");
    }
  }

  async function approve(application: BusinessApplication) {
    if (actionPendingID) {
      return;
    }
    setActionPendingID(application.id);
    setActionError(null);
    setNotice(null);
    try {
      const approval = await apiRequest(
        `/api/platform/applications/${encodeURIComponent(application.id)}/approve`,
        decodeApprovedApplication,
        { method: "POST" },
      );
      setNotice({
        message: `${approval.businessName} was approved.`,
        restaurantSlug: approval.slug,
      });
      setRefreshVersion((version) => version + 1);
    } catch (error: unknown) {
      setActionError(
        error instanceof ApiError
          ? error.message
          : "We couldn't approve this application. Try again.",
      );
    } finally {
      setActionPendingID(null);
    }
  }

  async function reject(
    event: FormEvent<HTMLFormElement>,
    application: BusinessApplication,
  ) {
    event.preventDefault();
    if (actionPendingID) {
      return;
    }
    const formData = new FormData(event.currentTarget);
    const reviewNote = String(formData.get("reviewNote") ?? "").trim();
    setActionPendingID(application.id);
    setActionError(null);
    setNotice(null);
    try {
      await apiRequest(
        `/api/platform/applications/${encodeURIComponent(application.id)}/reject`,
        decodeEmptyObject,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ reviewNote }),
        },
      );
      setNotice({ message: `${application.businessName} was declined.` });
      setRejectingID(null);
      setRefreshVersion((version) => version + 1);
    } catch (error: unknown) {
      setActionError(
        error instanceof ApiError
          ? error.message
          : "We couldn't decline this application. Try again.",
      );
    } finally {
      setActionPendingID(null);
    }
  }

  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <header className={styles.header}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
          <div className={styles.account}>
            {admin && (
              <span>
                Signed in as <strong>{admin.name}</strong>
              </span>
            )}
            <button
              className={styles.textButton}
              disabled={actionPendingID !== null}
              onClick={() => void signOut()}
              type="button"
            >
              Sign out
            </button>
          </div>
        </header>

        <section
          aria-labelledby="applications-title"
          className={styles.content}
        >
          <div className={styles.pageHeading}>
            <div>
              <h1 id="applications-title">Restaurant applications</h1>
              <p>Review business details and decide which pages go live.</p>
            </div>
            <label className={styles.filterField}>
              <span>Show</span>
              <select
                onChange={(event) => {
                  const value = event.currentTarget.value;
                  if (isApplicationListFilter(value)) {
                    setFilter(value);
                  }
                }}
                value={filter}
              >
                {applicationFilters.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </label>
          </div>

          {notice && (
            <p className={styles.notice} role="status">
              {notice.message}{" "}
              {notice.restaurantSlug && (
                <Link href={`/r/${encodeURIComponent(notice.restaurantSlug)}`}>
                  Open guest page
                </Link>
              )}
            </p>
          )}
          {actionError && (
            <p className={styles.errorMessage} role="alert">
              {actionError}
            </p>
          )}

          {isLoading ? (
            <p className={styles.loadingState} role="status">
              Loading applications…
            </p>
          ) : loadError ? (
            <section className={styles.emptyState} role="alert">
              <h2>Applications unavailable</h2>
              <p>{loadError}</p>
              <button
                className={styles.secondaryButton}
                onClick={() => setRefreshVersion((version) => version + 1)}
                type="button"
              >
                Try again
              </button>
            </section>
          ) : applications.length === 0 ? (
            <section className={styles.emptyState} role="status">
              <h2>No applications in this view</h2>
              <p>
                {filter === "PENDING"
                  ? "New restaurant applications will appear here."
                  : "Change the filter to see applications in another status."}
              </p>
            </section>
          ) : (
            <ol className={styles.applicationList}>
              {applications.map((application) => (
                <li className={styles.application} key={application.id}>
                  <div className={styles.applicationHeading}>
                    <div>
                      <h2>{application.businessName}</h2>
                      <p>
                        {application.businessType.toLowerCase()} ·{" "}
                        <code>/r/{application.slug}</code>
                      </p>
                    </div>
                    <span
                      className={styles.status}
                      data-status={application.status.toLowerCase()}
                    >
                      {application.status === "REJECTED"
                        ? "Not approved"
                        : application.status.toLowerCase()}
                    </span>
                  </div>

                  <dl className={styles.details}>
                    <div>
                      <dt>Owner</dt>
                      <dd>{application.ownerName}</dd>
                    </div>
                    <div>
                      <dt>Email</dt>
                      <dd>
                        <a href={`mailto:${application.ownerEmail}`}>
                          {application.ownerEmail}
                        </a>
                      </dd>
                    </div>
                    {application.address && (
                      <div>
                        <dt>Address</dt>
                        <dd>{application.address}</dd>
                      </div>
                    )}
                    {application.phone && (
                      <div>
                        <dt>Phone</dt>
                        <dd>
                          <a href={`tel:${application.phone}`}>
                            {application.phone}
                          </a>
                        </dd>
                      </div>
                    )}
                    <div>
                      <dt>Queue services</dt>
                      <dd>
                        {[
                          application.tableQueueEnabled && "Table waitlist",
                          application.pickupQueueEnabled && "Pickup orders",
                        ]
                          .filter(Boolean)
                          .join(" and ")}
                      </dd>
                    </div>
                    <div>
                      <dt>Submitted</dt>
                      <dd>{formatApplicationDate(application.createdAt)}</dd>
                    </div>
                  </dl>

                  {application.reviewNote && (
                    <p className={styles.reviewNote}>
                      Review note: {application.reviewNote}
                    </p>
                  )}

                  {application.status === "PENDING" && (
                    <div className={styles.reviewActions}>
                      <button
                        className={styles.primaryButton}
                        disabled={actionPendingID !== null}
                        onClick={() => void approve(application)}
                        type="button"
                      >
                        {actionPendingID === application.id
                          ? "Saving…"
                          : "Approve and publish"}
                      </button>
                      {rejectingID === application.id ? (
                        <form
                          className={styles.rejectForm}
                          onSubmit={(event) => void reject(event, application)}
                        >
                          <label className={styles.field}>
                            <span>Note for the applicant</span>
                            <textarea
                              maxLength={1000}
                              minLength={1}
                              name="reviewNote"
                              required
                              rows={3}
                            />
                          </label>
                          <div className={styles.rejectActions}>
                            <button
                              className={styles.secondaryButton}
                              disabled={actionPendingID !== null}
                              type="submit"
                            >
                              {actionPendingID === application.id
                                ? "Saving…"
                                : "Confirm decline"}
                            </button>
                            <button
                              className={styles.textButton}
                              disabled={actionPendingID !== null}
                              onClick={() => setRejectingID(null)}
                              type="button"
                            >
                              Cancel
                            </button>
                          </div>
                        </form>
                      ) : (
                        <button
                          className={styles.textButton}
                          disabled={actionPendingID !== null}
                          onClick={() => setRejectingID(application.id)}
                          type="button"
                        >
                          Decline
                        </button>
                      )}
                    </div>
                  )}
                </li>
              ))}
            </ol>
          )}
        </section>
      </div>
    </main>
  );
}

function formatApplicationDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
