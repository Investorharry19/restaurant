"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError, apiRequest } from "../../lib/api/client";
import {
  decodeEmptyObject,
  decodeOwnerAccountSession,
} from "../../lib/api/applications";
import styles from "./owner-account.module.css";

type AccountMode = "register" | "sign-in";

function getOwnerReturnPath(): string {
  const requestedPath = new URLSearchParams(window.location.search).get("next");
  return requestedPath && /^\/workspace\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(requestedPath)
    ? requestedPath
    : "/onboarding";
}

export function OwnerAccount() {
  const router = useRouter();
  const [mode, setMode] = useState<AccountMode>("register");
  const [email, setEmail] = useState("");
  const [isCheckingSession, setIsCheckingSession] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [verificationSent, setVerificationSent] = useState(false);
  const [showResend, setShowResend] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    void apiRequest("/api/owner/auth/me", decodeOwnerAccountSession, {
      signal: controller.signal,
    })
      .then(() => {
        if (active) {
          router.replace(getOwnerReturnPath());
        }
      })
      .catch((error: unknown) => {
        if (
          active &&
          (!(error instanceof ApiError) || error.code !== "OWNER_AUTH_REQUIRED")
        ) {
          setErrorMessage(
            "We couldn't check your account. You can still try signing in or creating one.",
          );
        }
      })
      .finally(() => {
        if (active) {
          setIsCheckingSession(false);
        }
      });

    return () => {
      active = false;
      controller.abort();
    };
  }, [router]);

  async function submitAccount(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isSubmitting) {
      return;
    }

    const formData = new FormData(event.currentTarget);
    const submittedEmail = String(formData.get("email") ?? "")
      .trim()
      .toLowerCase();
    setEmail(submittedEmail);
    setMessage(null);
    setErrorMessage(null);
    setShowResend(false);
    setIsSubmitting(true);

    try {
      if (mode === "register") {
        await apiRequest("/api/public/accounts", decodeEmptyObject, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            name: String(formData.get("name") ?? "").trim(),
            email: submittedEmail,
            password: String(formData.get("password") ?? ""),
          }),
        });
        setVerificationSent(true);
        setMessage(
          "If this email can be registered, a verification link is on its way.",
        );
      } else {
        await apiRequest("/api/owner/auth/login", decodeOwnerAccountSession, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            email: submittedEmail,
            password: String(formData.get("password") ?? ""),
          }),
        });
        router.replace(getOwnerReturnPath());
      }
    } catch (error: unknown) {
      if (error instanceof ApiError) {
        if (error.code === "OWNER_EMAIL_NOT_VERIFIED") {
          setShowResend(true);
          setErrorMessage("Verify your email address before signing in.");
        } else if (error.code === "OWNER_AUTH_INVALID_CREDENTIALS") {
          setErrorMessage(
            "That email and password combination isn't recognized.",
          );
        } else if (error.code === "EMAIL_DELIVERY_UNAVAILABLE") {
          setErrorMessage(
            "We couldn't send the verification email. Try again shortly.",
          );
          setShowResend(true);
        } else if (error.code === "OWNER_ACCOUNT_INVALID") {
          setErrorMessage(error.message);
        } else {
          setErrorMessage("We couldn't complete that request. Try again.");
        }
      } else {
        setErrorMessage(
          "We couldn't complete that request. Check your connection and try again.",
        );
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  async function resendVerification() {
    if (!email || isSubmitting) {
      return;
    }
    setErrorMessage(null);
    setMessage(null);
    setIsSubmitting(true);
    try {
      await apiRequest(
        "/api/public/accounts/verification/resend",
        decodeEmptyObject,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email }),
        },
      );
      setMessage(
        "If this account needs verification, a new link is on its way.",
      );
      setShowResend(false);
    } catch (error: unknown) {
      setErrorMessage(
        error instanceof ApiError && error.code === "EMAIL_DELIVERY_UNAVAILABLE"
          ? "We couldn't send the verification email. Try again shortly."
          : "We couldn't resend the link. Check your connection and try again.",
      );
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className={styles.page}>
      <a className={styles.skipLink} href="#account-title">
        Skip to account form
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

        <section
          aria-labelledby="account-title"
          className={styles.accountPanel}
        >
          <h1 id="account-title">Start with your owner account.</h1>
          <p className={styles.intro}>
            Create an account and verify your email before sharing your business
            details.
          </p>

          {isCheckingSession ? (
            <p className={styles.status} role="status">
              Checking your account…
            </p>
          ) : verificationSent ? (
            <div className={styles.confirmation} role="status">
              <h2>Check your inbox</h2>
              <p>
                If {email} can be registered, a verification link is on its way.
                If it arrives, open it to verify your email and continue.
              </p>
              {message && (
                <p className={styles.message} role="status">
                  {message}
                </p>
              )}
              <button
                className={styles.textButton}
                disabled={isSubmitting}
                onClick={() => void resendVerification()}
                type="button"
              >
                {isSubmitting ? "Sending…" : "Send another verification link"}
              </button>
              <button
                className={styles.textButton}
                onClick={() => {
                  setVerificationSent(false);
                  setEmail("");
                  setMessage(null);
                  setErrorMessage(null);
                }}
                type="button"
              >
                Use a different email address
              </button>
              <button
                className={styles.textButton}
                onClick={() => {
                  setVerificationSent(false);
                  setMode("sign-in");
                  setMessage(null);
                  setErrorMessage(null);
                }}
                type="button"
              >
                Already have an owner account? Sign in
              </button>
            </div>
          ) : (
            <>
              <div
                aria-label="Owner account options"
                className={styles.modeTabs}
                role="group"
              >
                <button
                  aria-pressed={mode === "register"}
                  className={mode === "register" ? styles.selectedTab : ""}
                  disabled={isSubmitting}
                  onClick={() => {
                    setMode("register");
                    setMessage(null);
                    setErrorMessage(null);
                    setShowResend(false);
                  }}
                  type="button"
                >
                  Create account
                </button>
                <button
                  aria-pressed={mode === "sign-in"}
                  className={mode === "sign-in" ? styles.selectedTab : ""}
                  disabled={isSubmitting}
                  onClick={() => {
                    setMode("sign-in");
                    setMessage(null);
                    setErrorMessage(null);
                    setShowResend(false);
                  }}
                  type="button"
                >
                  Sign in
                </button>
              </div>

              <form
                className={styles.form}
                id="account-form"
                onSubmit={submitAccount}
              >
                {mode === "register" && (
                  <label className={styles.field}>
                    <span>Your name</span>
                    <input
                      autoComplete="name"
                      maxLength={120}
                      name="name"
                      required
                    />
                  </label>
                )}
                <label className={styles.field}>
                  <span>Email</span>
                  <input
                    autoComplete="email"
                    maxLength={320}
                    name="email"
                    onChange={(event) => setEmail(event.currentTarget.value)}
                    required
                    type="email"
                    value={email}
                  />
                </label>
                <label className={styles.field}>
                  <span>Password</span>
                  <input
                    autoComplete={
                      mode === "register" ? "new-password" : "current-password"
                    }
                    maxLength={1024}
                    minLength={mode === "register" ? 12 : undefined}
                    name="password"
                    required
                    type="password"
                  />
                  {mode === "register" && (
                    <small>Use at least 12 characters.</small>
                  )}
                </label>

                {errorMessage && (
                  <p className={styles.errorMessage} role="alert">
                    {errorMessage}
                  </p>
                )}
                {message && (
                  <p className={styles.message} role="status">
                    {message}
                  </p>
                )}

                <button
                  className={styles.primaryButton}
                  disabled={isSubmitting}
                  type="submit"
                >
                  {isSubmitting
                    ? mode === "register"
                      ? "Creating account…"
                      : "Signing in…"
                    : mode === "register"
                      ? "Create account"
                      : "Sign in"}
                </button>
                {showResend && (
                  <button
                    className={styles.textButton}
                    disabled={isSubmitting}
                    onClick={() => void resendVerification()}
                    type="button"
                  >
                    Send a new verification link
                  </button>
                )}
              </form>
            </>
          )}
        </section>

        <footer className={styles.footer}>
          <p>
            Your business application is only available after email
            verification.
          </p>
          <Link href="/">Restaurant queues</Link>
        </footer>
      </div>
    </main>
  );
}
