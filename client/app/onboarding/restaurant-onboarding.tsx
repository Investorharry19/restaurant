"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest } from "../../lib/api/client";
import {
  decodeCreatedApplication,
  decodeEmptyObject,
  decodeOwnerAccountSession,
  type OwnerAccountSession,
} from "../../lib/api/applications";
import styles from "./onboarding.module.css";

function slugFromName(value: string): string {
  return value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function getApplicationError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === "APPLICATION_CONFLICT") {
      return "That business URL or owner email is already in use. Change one and try again.";
    }
    if (error.code === "APPLICATION_RATE_LIMITED") {
      return "Too many applications were submitted from this connection. Try again later.";
    }
    if (error.code === "APPLICATION_INVALID") {
      return error.message;
    }
  }

  return "We couldn't send your application. Check your connection and try again.";
}

export function RestaurantOnboarding() {
  const router = useRouter();
  const [owner, setOwner] = useState<OwnerAccountSession | null>(null);
  const [isCheckingAccount, setIsCheckingAccount] = useState(true);
  const [accountCheckError, setAccountCheckError] = useState<string | null>(
    null,
  );
  const [accountCheckAttempt, setAccountCheckAttempt] = useState(0);
  const [businessName, setBusinessName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    void apiRequest("/api/owner/auth/me", decodeOwnerAccountSession, {
      signal: controller.signal,
    })
      .then((identity) => {
        if (active) {
          setOwner(identity);
        }
      })
      .catch((error: unknown) => {
        if (!active) {
          return;
        }
        if (error instanceof ApiError && error.code === "OWNER_AUTH_REQUIRED") {
          router.replace("/account");
        } else {
          setAccountCheckError(
            "We couldn't check your verified account. Try again or sign in.",
          );
        }
      })
      .finally(() => {
        if (active) {
          setIsCheckingAccount(false);
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [accountCheckAttempt, router]);

  async function submitApplication(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }

    const formData = new FormData(event.currentTarget);
    const tableQueueEnabled = formData.get("tableQueueEnabled") === "on";
    const pickupQueueEnabled = formData.get("pickupQueueEnabled") === "on";
    if (!tableQueueEnabled && !pickupQueueEnabled) {
      setErrorMessage("Choose at least one queue service to continue.");
      return;
    }

    const body = {
      businessName: String(formData.get("businessName") ?? "").trim(),
      businessType: String(formData.get("businessType") ?? ""),
      slug: String(formData.get("slug") ?? "").trim(),
      address: String(formData.get("address") ?? "").trim(),
      phone: String(formData.get("phone") ?? "").trim(),
      tableQueueEnabled,
      pickupQueueEnabled,
    };

    setErrorMessage(null);
    setIsSubmitting(true);
    try {
      const application = await apiRequest(
        "/api/owner/applications",
        decodeCreatedApplication,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
      );
      router.replace(
        `/application/${encodeURIComponent(application.statusToken)}`,
      );
    } catch (error: unknown) {
      if (error instanceof ApiError && error.code === "OWNER_AUTH_REQUIRED") {
        router.replace("/account");
        return;
      }
      setErrorMessage(getApplicationError(error));
    } finally {
      setIsSubmitting(false);
    }
  }

  async function signOut() {
    try {
      await apiRequest("/api/owner/auth/logout", decodeEmptyObject, {
        method: "POST",
      });
      router.replace("/account");
    } catch {
      setErrorMessage("We couldn't sign you out. Try again.");
    }
  }

  if (isCheckingAccount || !owner) {
    return (
      <main className={styles.page}>
        <div className={styles.statusShell}>
          <div className={styles.statusHeader}>
            <h1>Owner account</h1>
            <p>
              {isCheckingAccount
                ? "Checking your verified account…"
                : (accountCheckError ??
                  "A verified owner account is required. Opening account sign-in…")}
            </p>
          </div>
          {accountCheckError && (
            <div className={styles.statusActions}>
              <button
                className={styles.refreshButton}
                onClick={() => setAccountCheckAttempt((attempt) => attempt + 1)}
                type="button"
              >
                Try again
              </button>
              <Link href="/account">Sign in</Link>
            </div>
          )}
        </div>
      </main>
    );
  }

  return (
    <main className={styles.page}>
      <a className={styles.skipLink} href="#application-form">
        Skip to application
      </a>
      <div className={styles.shell}>
        <header className={styles.header}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
          <Link className={styles.headerLink} href="/">
            Back to home
          </Link>
        </header>

        <div className={styles.content}>
          <section aria-labelledby="page-title" className={styles.intro}>
            <h1 id="page-title">Bring your restaurant online.</h1>
            <p>
              Set up a simple way for guests to join your table waitlist or
              follow pickup orders from their phones.
            </p>

            <div className={styles.nextSteps}>
              <h2>What happens next</h2>
              <ol>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>Verify your owner account</strong>
                    <small>
                      Your verified email keeps your application connected to
                      you.
                    </small>
                  </span>
                </li>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>Share your business details</strong>
                    <small>Choose the queue services you need.</small>
                  </span>
                </li>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>We review your application</strong>
                    <small>Use your private link to check its status.</small>
                  </span>
                </li>
              </ol>
            </div>
          </section>

          <section
            aria-labelledby="form-title"
            className={styles.formSection}
            id="application-form"
          >
            <div className={styles.formHeading}>
              <h2 id="form-title">Business application</h2>
              <p>Fields marked required need to be completed.</p>
            </div>

            <div className={styles.ownerAccount}>
              <div>
                <strong>Verified owner account</strong>
                <span>
                  {owner.name} · {owner.email}
                </span>
              </div>
              <button
                className={styles.accountAction}
                onClick={() => void signOut()}
                type="button"
              >
                Sign out
              </button>
            </div>

            <form className={styles.form} onSubmit={submitApplication}>
              <fieldset className={styles.fieldset}>
                <legend>Business details</legend>
                <div className={styles.fieldGrid}>
                  <label className={styles.field}>
                    <span>Business name</span>
                    <input
                      autoComplete="organization"
                      maxLength={120}
                      name="businessName"
                      onChange={(event) => {
                        const nextName = event.currentTarget.value;
                        setBusinessName(nextName);
                        if (!slugEdited) {
                          setSlug(slugFromName(nextName));
                        }
                      }}
                      required
                      value={businessName}
                    />
                  </label>

                  <label className={styles.field}>
                    <span>Business type</span>
                    <select defaultValue="RESTAURANT" name="businessType">
                      <option value="RESTAURANT">Restaurant</option>
                      <option value="BAKERY">Bakery</option>
                      <option value="EATERY">Eatery</option>
                    </select>
                  </label>

                  <label className={styles.field}>
                    <span>Restaurant page address</span>
                    <span className={styles.slugInput}>
                      <span aria-hidden="true">/r/</span>
                      <input
                        autoCapitalize="none"
                        autoComplete="off"
                        maxLength={120}
                        name="slug"
                        onChange={(event) => {
                          setSlugEdited(true);
                          setSlug(event.currentTarget.value.toLowerCase());
                        }}
                        pattern="[a-z0-9]+(-[a-z0-9]+)*"
                        required
                        spellCheck={false}
                        value={slug}
                      />
                    </span>
                    <small>
                      Lowercase letters, numbers, and single hyphens.
                    </small>
                  </label>

                  <label className={styles.field}>
                    <span>
                      Address <em>Optional</em>
                    </span>
                    <input
                      autoComplete="street-address"
                      maxLength={500}
                      name="address"
                    />
                  </label>

                  <label className={styles.field}>
                    <span>
                      Phone <em>Optional</em>
                    </span>
                    <input
                      autoComplete="tel"
                      maxLength={50}
                      name="phone"
                      type="tel"
                    />
                  </label>
                </div>
              </fieldset>

              <fieldset className={styles.fieldset}>
                <legend>Queue services</legend>
                <p className={styles.fieldsetHint}>
                  Choose at least one service. You can enable both.
                </p>
                <div className={styles.queueChoices}>
                  <label className={styles.queueChoice}>
                    <input
                      defaultChecked
                      name="tableQueueEnabled"
                      onChange={() => setErrorMessage(null)}
                      type="checkbox"
                    />
                    <span>
                      <strong>Table waitlist</strong>
                      <small>Guests join the line for a table.</small>
                    </span>
                  </label>
                  <label className={styles.queueChoice}>
                    <input
                      defaultChecked
                      name="pickupQueueEnabled"
                      onChange={() => setErrorMessage(null)}
                      type="checkbox"
                    />
                    <span>
                      <strong>Pickup orders</strong>
                      <small>Guests follow a pickup order with a code.</small>
                    </span>
                  </label>
                </div>
              </fieldset>

              {errorMessage && (
                <p className={styles.errorMessage} role="alert">
                  {errorMessage}
                </p>
              )}

              <div className={styles.formActions}>
                <button
                  className={styles.primaryButton}
                  disabled={isSubmitting}
                  type="submit"
                >
                  {isSubmitting ? "Sending application…" : "Send application"}
                </button>
                <p>Your password is never shown to reviewers.</p>
              </div>
            </form>
          </section>
        </div>

        <footer className={styles.footer}>
          <p>Applying does not make your restaurant public automatically.</p>
          <Link href="/">Restaurant queues</Link>
        </footer>
      </div>
    </main>
  );
}
