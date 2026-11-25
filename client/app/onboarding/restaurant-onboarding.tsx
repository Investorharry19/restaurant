"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest } from "../../lib/api/client";
import {
  decodeOwnerApplicationDraft,
  decodeCreatedApplication,
  decodeEmptyObject,
  decodeOwnerAccountSession,
  type OwnerApplicationDraft,
  type OwnerAccountSession,
} from "../../lib/api/applications";
import styles from "./onboarding.module.css";

const maxLogoBytes = 512 * 1024;

function ownerAccountPath(resubmitToken?: string): string {
  const nextPath = resubmitToken
    ? `/onboarding?resubmit=${encodeURIComponent(resubmitToken)}`
    : "/onboarding";
  return `/account?next=${encodeURIComponent(nextPath)}`;
}

function fileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result !== "string") {
        reject(new TypeError("The selected logo could not be read."));
        return;
      }
      const separator = reader.result.indexOf(",");
      if (separator < 0) {
        reject(new TypeError("The selected logo could not be read."));
        return;
      }
      resolve(reader.result.slice(separator + 1));
    };
    reader.onerror = () =>
      reject(new Error("The selected logo could not be read."));
    reader.readAsDataURL(file);
  });
}

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

export function RestaurantOnboarding({
  resubmitToken,
}: {
  resubmitToken?: string;
}) {
  const router = useRouter();
  const [owner, setOwner] = useState<OwnerAccountSession | null>(null);
  const [isCheckingAccount, setIsCheckingAccount] = useState(true);
  const [accountCheckError, setAccountCheckError] = useState<string | null>(
    null,
  );
  const [accountCheckAttempt, setAccountCheckAttempt] = useState(0);
  const [resubmitDraft, setResubmitDraft] =
    useState<OwnerApplicationDraft | null>(null);
  const [isLoadingDraft, setIsLoadingDraft] = useState(Boolean(resubmitToken));
  const [draftError, setDraftError] = useState<string | null>(null);
  const [draftAttempt, setDraftAttempt] = useState(0);
  const [businessName, setBusinessName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [logoFile, setLogoFile] = useState<File | null>(null);
  const [logoPreviewURL, setLogoPreviewURL] = useState<string | null>(null);
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
          router.replace(ownerAccountPath(resubmitToken));
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
  }, [accountCheckAttempt, resubmitToken, router]);

  useEffect(() => {
    if (!owner || !resubmitToken) {
      return;
    }

    const controller = new AbortController();
    let active = true;

    void apiRequest(
      `/api/owner/applications/${encodeURIComponent(resubmitToken)}`,
      decodeOwnerApplicationDraft,
      { signal: controller.signal },
    )
      .then((draft) => {
        if (active) {
          setResubmitDraft(draft);
          setBusinessName(draft.businessName);
          setSlug(draft.slug);
          setSlugEdited(true);
        }
      })
      .catch((error: unknown) => {
        if (!active) {
          return;
        }
        if (error instanceof ApiError && error.code === "OWNER_AUTH_REQUIRED") {
          router.replace(ownerAccountPath(resubmitToken));
          return;
        }
        setDraftError(
          error instanceof ApiError && error.status === 404
            ? "This rejected application is unavailable for editing. Sign in with the owner account that submitted it."
            : "We couldn't load your rejected application. Try again.",
        );
      })
      .finally(() => {
        if (active) {
          setIsLoadingDraft(false);
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [draftAttempt, owner, resubmitToken, router]);

  useEffect(
    () => () => {
      if (logoPreviewURL) {
        URL.revokeObjectURL(logoPreviewURL);
      }
    },
    [logoPreviewURL],
  );

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

    const body: {
      businessName: string;
      businessType: string;
      slug: string;
      address: string;
      phone: string;
      tableQueueEnabled: boolean;
      pickupQueueEnabled: boolean;
      logoBase64?: string;
      logoContentType?: string;
    } = {
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
      if (logoFile) {
        body.logoBase64 = await fileAsBase64(logoFile);
        body.logoContentType = logoFile.type;
      }
      const application = await apiRequest(
        resubmitToken
          ? `/api/owner/applications/${encodeURIComponent(resubmitToken)}`
          : "/api/owner/applications",
        decodeCreatedApplication,
        {
          method: resubmitToken ? "PATCH" : "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
      );
      router.replace(
        `/application/${encodeURIComponent(application.statusToken)}`,
      );
    } catch (error: unknown) {
      if (error instanceof ApiError && error.code === "OWNER_AUTH_REQUIRED") {
        router.replace(ownerAccountPath(resubmitToken));
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

  if (
    isCheckingAccount ||
    !owner ||
    isLoadingDraft ||
    (Boolean(resubmitToken) && !resubmitDraft)
  ) {
    return (
      <main className={styles.page}>
        <div className={styles.statusShell}>
          <div className={styles.statusHeader}>
            <h1>Owner account</h1>
            <p>
              {isCheckingAccount
                ? "Checking your verified account…"
                : draftError
                  ? draftError
                  : isLoadingDraft
                    ? "Loading your application for editing…"
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
          {draftError && (
            <div className={styles.statusActions}>
              <button
                className={styles.refreshButton}
                onClick={() => {
                  setDraftError(null);
                  setIsLoadingDraft(true);
                  setDraftAttempt((attempt) => attempt + 1);
                }}
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
            <h1 id="page-title">
              {resubmitToken
                ? "Update your business application."
                : "Bring your restaurant online."}
            </h1>
            <p>
              {resubmitToken
                ? "Review the feedback, update the details that need attention, and send your application back for review."
                : "Set up a simple way for guests to join your table waitlist or follow pickup orders from their phones."}
            </p>

            <div className={styles.nextSteps}>
              <h2>
                {resubmitToken ? "Your application" : "What happens next"}
              </h2>
              <ol>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>
                      {resubmitToken
                        ? "Address the review feedback"
                        : "Verify your owner account"}
                    </strong>
                    <small>
                      {resubmitToken
                        ? (resubmitDraft?.reviewNote ??
                          "Update the business details before resubmitting.")
                        : "Your verified email keeps your application connected to you."}
                    </small>
                  </span>
                </li>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>
                      {resubmitToken
                        ? "Update business details"
                        : "Share your business details"}
                    </strong>
                    <small>
                      {resubmitToken
                        ? "Keep the details that are already correct."
                        : "Choose the queue services you need."}
                    </small>
                  </span>
                </li>
                <li>
                  <span className={styles.stepMarker} aria-hidden="true" />
                  <span>
                    <strong>
                      {resubmitToken
                        ? "We review it again"
                        : "We review your application"}
                    </strong>
                    <small>
                      Use the same private link to check its status.
                    </small>
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
              <h2 id="form-title">
                {resubmitToken ? "Update application" : "Business application"}
              </h2>
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
                    <select
                      defaultValue={resubmitDraft?.businessType ?? "RESTAURANT"}
                      name="businessType"
                    >
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
                      Restaurant logo <em>Optional</em>
                    </span>
                    {resubmitDraft?.logoUrl && !logoPreviewURL && (
                      <span className={styles.logoPreview}>
                        <Image
                          alt="Current restaurant logo"
                          className={styles.logoImage}
                          height="64"
                          src={resubmitDraft.logoUrl}
                          unoptimized
                          width="64"
                        />
                        <small>Current logo</small>
                      </span>
                    )}
                    {logoPreviewURL && (
                      <span className={styles.logoPreview}>
                        <Image
                          alt="Selected restaurant logo preview"
                          className={styles.logoImage}
                          height="64"
                          src={logoPreviewURL}
                          unoptimized
                          width="64"
                        />
                        <small>New logo preview</small>
                      </span>
                    )}
                    <input
                      accept="image/png,image/jpeg"
                      name="restaurantLogo"
                      onChange={(event) => {
                        const file = event.currentTarget.files?.[0] ?? null;
                        if (
                          file &&
                          (file.size > maxLogoBytes ||
                            (file.type !== "image/png" &&
                              file.type !== "image/jpeg"))
                        ) {
                          event.currentTarget.value = "";
                          setLogoFile(null);
                          setLogoPreviewURL(null);
                          setErrorMessage(
                            "Choose a PNG or JPEG logo no larger than 512 KB.",
                          );
                          return;
                        }
                        setErrorMessage(null);
                        setLogoFile(file);
                        setLogoPreviewURL(
                          file ? URL.createObjectURL(file) : null,
                        );
                      }}
                      type="file"
                    />
                    <small>
                      PNG or JPEG, 512 KB maximum. Square images work best.
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
                      defaultValue={resubmitDraft?.address ?? ""}
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
                      defaultValue={resubmitDraft?.phone ?? ""}
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
                      defaultChecked={resubmitDraft?.tableQueueEnabled ?? true}
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
                      defaultChecked={resubmitDraft?.pickupQueueEnabled ?? true}
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
                  {isSubmitting
                    ? resubmitToken
                      ? "Resubmitting…"
                      : "Sending application…"
                    : resubmitToken
                      ? "Resubmit application"
                      : "Send application"}
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
