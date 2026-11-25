"use client";

import { useEffect, useState } from "react";
import { ApiError, apiRequest } from "../../../lib/api/client";
import {
  decodeTicketStatus,
  type TicketStatus,
  type TicketStatusView,
} from "../../../lib/api/tickets";
import styles from "./ticket-status.module.css";

type LoadedTicket = {
  token: string;
  ticket: TicketStatusView;
};

function getStatusErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 404) {
      return "This ticket link isn't available. Check that you have the full link, or ask the restaurant team for help.";
    }
    if (error.status === 429) {
      return "Too many status checks. Wait a moment, then try again.";
    }
  }

  return "We couldn't load this ticket right now. Check your connection and try again.";
}

function statusLabel(status: TicketStatus): string {
  switch (status) {
    case "WAITING":
      return "Waiting";
    case "CALLED":
      return "Called";
    case "ARRIVED":
      return "Arrival confirmed";
    case "SEATED":
      return "Seated";
    case "NO_SHOW":
      return "No-show";
    case "PREPARING":
      return "Being prepared";
    case "READY":
      return "Ready";
    case "PICKED_UP":
      return "Picked up";
    case "CANCELLED":
      return "Cancelled";
    case "EXPIRED":
      return "Expired";
  }
}

function formatUpdatedAt(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

type PushSubscriptionPayload = {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
};

type PushPreferenceState =
  | "checking"
  | "ready"
  | "enabled"
  | "denied"
  | "unavailable"
  | "unsupported"
  | "busy"
  | "error";

function decodeVapidKey(data: unknown): string {
  if (
    typeof data !== "object" ||
    data === null ||
    typeof (data as Record<string, unknown>).publicKey !== "string"
  ) {
    throw new TypeError("The push notification key response is invalid.");
  }
  return (data as Record<string, string>).publicKey;
}

function decodeEmpty(data: unknown): Record<string, never> {
  if (typeof data !== "object" || data === null || Array.isArray(data)) {
    throw new TypeError("The push notification response is invalid.");
  }
  return {};
}

function applicationServerKey(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
  const binary = window.atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes.buffer;
}

function serializeSubscription(
  subscription: PushSubscription,
): PushSubscriptionPayload {
  const json = subscription.toJSON();
  if (
    !subscription.endpoint ||
    typeof json.keys?.p256dh !== "string" ||
    typeof json.keys.auth !== "string"
  ) {
    throw new TypeError(
      "The browser returned an incomplete push subscription.",
    );
  }
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh: json.keys.p256dh, auth: json.keys.auth },
  };
}

async function savePushSubscription(
  token: string,
  subscription: PushSubscription,
): Promise<void> {
  await apiRequest(
    `/api/public/tickets/${encodeURIComponent(token)}/push-subscriptions`,
    decodeEmpty,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(serializeSubscription(subscription)),
    },
  );
}

async function deletePushSubscription(
  token: string,
  endpoint: string,
): Promise<void> {
  await apiRequest(
    `/api/public/tickets/${encodeURIComponent(token)}/push-subscriptions`,
    decodeEmpty,
    {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ endpoint }),
    },
  );
}

function TicketPushControls({
  token,
  status,
}: {
  token: string;
  status: TicketStatus;
}) {
  const [pushState, setPushState] = useState<PushPreferenceState>("checking");
  const [message, setMessage] = useState<string | null>(null);
  const [vapidKey, setVapidKey] = useState<string | null>(null);

  const canReceiveUpdates = [
    "WAITING",
    "CALLED",
    "ARRIVED",
    "PREPARING",
    "READY",
  ].includes(status);

  useEffect(() => {
    if (!canReceiveUpdates) {
      return;
    }

    let active = true;
    let registration: ServiceWorkerRegistration | null = null;

    const showSetupState = () => {
      if (!active) {
        return;
      }
      if (
        !("Notification" in window) ||
        !("serviceWorker" in navigator) ||
        !("PushManager" in window)
      ) {
        setPushState("unsupported");
        return;
      }
      setPushState(Notification.permission === "denied" ? "denied" : "ready");
    };

    const syncCurrentSubscription = async (previousEndpoint?: string) => {
      if (
        !registration ||
        !("Notification" in window) ||
        Notification.permission === "denied"
      ) {
        return false;
      }
      const subscription = await registration.pushManager.getSubscription();
      if (previousEndpoint && previousEndpoint !== subscription?.endpoint) {
        await deletePushSubscription(token, previousEndpoint);
      }
      if (subscription) {
        await savePushSubscription(token, subscription);
        if (active) {
          setPushState("enabled");
          setMessage(null);
        }
        return true;
      }
      return false;
    };

    const onServiceWorkerMessage = (event: MessageEvent<unknown>) => {
      if (
        typeof event.data !== "object" ||
        event.data === null ||
        !("type" in event.data)
      ) {
        return;
      }
      if (event.data.type === "PUSH_SUBSCRIPTION_REFRESH_FAILED") {
        setPushState("error");
        setMessage(
          "This device could not renew ticket alerts. Try enabling them again.",
        );
        return;
      }
      if (event.data.type !== "PUSH_SUBSCRIPTION_CHANGED") {
        return;
      }
      const previousEndpoint =
        "previousEndpoint" in event.data &&
        typeof event.data.previousEndpoint === "string"
          ? event.data.previousEndpoint
          : undefined;
      void syncCurrentSubscription(previousEndpoint).catch(() => {
        if (active) {
          setPushState("error");
          setMessage(
            "We could not refresh this device's alert subscription. Try again.",
          );
        }
      });
    };

    navigator.serviceWorker?.addEventListener(
      "message",
      onServiceWorkerMessage,
    );

    async function preparePush() {
      if (
        !("Notification" in window) ||
        !("serviceWorker" in navigator) ||
        !("PushManager" in window)
      ) {
        showSetupState();
        return;
      }
      if (Notification.permission === "denied") {
        showSetupState();
        return;
      }
      try {
        registration = await navigator.serviceWorker.register("/sw.js", {
          scope: "/",
        });
        const key = await apiRequest(
          "/api/public/push/vapid-key",
          decodeVapidKey,
        );
        if (active) {
          setVapidKey(key);
        }
        const hasSubscription = await syncCurrentSubscription();
        if (active && !hasSubscription) {
          showSetupState();
        }
      } catch (error: unknown) {
        if (active) {
          setPushState(
            error instanceof ApiError && error.status === 503
              ? "unavailable"
              : "error",
          );
          setMessage(
            error instanceof ApiError
              ? error.message
              : "We could not prepare alerts on this device. Try again.",
          );
        }
      }
    }

    void preparePush();
    return () => {
      active = false;
      navigator.serviceWorker?.removeEventListener(
        "message",
        onServiceWorkerMessage,
      );
    };
  }, [canReceiveUpdates, token]);

  if (!canReceiveUpdates) {
    return null;
  }

  async function checkPushAvailability() {
    setPushState("busy");
    setMessage(null);
    if (
      !("Notification" in window) ||
      !("serviceWorker" in navigator) ||
      !("PushManager" in window)
    ) {
      setPushState("unsupported");
      return;
    }
    if (Notification.permission === "denied") {
      setPushState("denied");
      return;
    }
    try {
      const key = await apiRequest(
        "/api/public/push/vapid-key",
        decodeVapidKey,
      );
      setVapidKey(key);
      const registration = await navigator.serviceWorker.register("/sw.js", {
        scope: "/",
      });
      const subscription = await registration.pushManager.getSubscription();
      if (subscription && Notification.permission === "granted") {
        await savePushSubscription(token, subscription);
        setPushState("enabled");
      } else {
        setPushState("ready");
      }
    } catch (error: unknown) {
      setPushState(
        error instanceof ApiError && error.status === 503
          ? "unavailable"
          : "error",
      );
      setMessage(
        error instanceof ApiError
          ? error.message
          : "We could not check ticket alerts. Try again.",
      );
    }
  }

  async function enablePush() {
    setPushState("busy");
    setMessage(null);
    try {
      if (
        !("Notification" in window) ||
        !("serviceWorker" in navigator) ||
        !("PushManager" in window)
      ) {
        setPushState("unsupported");
        return;
      }

      let permission = Notification.permission;
      if (permission === "default") {
        permission = await Notification.requestPermission();
      }
      if (permission === "denied") {
        setPushState("denied");
        return;
      }
      if (permission !== "granted") {
        setPushState("ready");
        return;
      }

      const registration =
        (await navigator.serviceWorker.getRegistration("/")) ??
        (await navigator.serviceWorker.register("/sw.js", { scope: "/" }));
      const existing = await registration.pushManager.getSubscription();
      let subscription = existing;
      if (!subscription) {
        if (!vapidKey) {
          setPushState("unavailable");
          setMessage("Ticket alerts are not configured for this service.");
          return;
        }
        subscription = await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: applicationServerKey(vapidKey),
        });
      }
      await savePushSubscription(token, subscription);
      setPushState("enabled");
    } catch (error: unknown) {
      setPushState("error");
      setMessage(
        error instanceof ApiError
          ? error.message
          : "We could not enable alerts on this device. Try again.",
      );
    }
  }

  async function disablePush() {
    setPushState("busy");
    setMessage(null);
    try {
      const registration = await navigator.serviceWorker.getRegistration("/");
      const subscription = await registration?.pushManager.getSubscription();
      if (subscription) {
        await deletePushSubscription(token, subscription.endpoint);
        try {
          await subscription.unsubscribe();
        } catch {
          setPushState("ready");
          setMessage(
            "Alerts are disabled for this ticket, but this browser could not remove its local subscription. You can remove it in browser settings.",
          );
          return;
        }
      }
      setPushState("ready");
    } catch (error: unknown) {
      setPushState("error");
      setMessage(
        error instanceof ApiError
          ? error.message
          : "We could not disable alerts on this device. Try again.",
      );
    }
  }

  return (
    <section
      aria-labelledby="ticket-alerts-heading"
      className={styles.pushPreference}
    >
      <h2 id="ticket-alerts-heading">Get ticket alerts</h2>
      <p>
        Get a notification when the restaurant calls your table or your pickup
        order is ready. You can turn alerts off at any time.
      </p>
      {pushState === "enabled" ? (
        <button
          className={styles.refreshButton}
          onClick={() => void disablePush()}
          type="button"
        >
          Turn off alerts
        </button>
      ) : pushState === "checking" || pushState === "busy" ? (
        <p aria-live="polite" role="status">
          {pushState === "checking"
            ? "Checking this device..."
            : "Updating alerts..."}
        </p>
      ) : pushState === "denied" ? (
        <p role="status">
          Alerts are blocked by your browser. Change this site&apos;s
          notification permission in browser settings to enable them.
        </p>
      ) : pushState === "unsupported" ? (
        <p role="status">
          This browser does not support ticket notifications. Keep this page
          open to see live status updates.
        </p>
      ) : pushState === "unavailable" ? (
        <>
          <p role="status">
            Ticket alerts are not available right now. Keep this page open to
            see live status updates.
          </p>
          {message && (
            <p className={styles.pushWarning} role="alert">
              {message}
            </p>
          )}
          <button
            className={styles.refreshButton}
            onClick={() => void checkPushAvailability()}
            type="button"
          >
            Check alerts again
          </button>
        </>
      ) : (
        <>
          {message && (
            <p className={styles.pushWarning} role="alert">
              {message}
            </p>
          )}
          <button
            className={styles.refreshButton}
            onClick={() =>
              pushState === "error"
                ? void checkPushAvailability()
                : void enablePush()
            }
            type="button"
          >
            {pushState === "error"
              ? "Check alerts again"
              : "Enable ticket alerts"}
          </button>
        </>
      )}
    </section>
  );
}

export function TicketStatusPage({ token }: { token: string }) {
  const [loadedTicket, setLoadedTicket] = useState<LoadedTicket | null>(null);
  const [requestState, setRequestState] = useState<{
    token: string;
    isLoading: boolean;
    errorMessage: string | null;
  }>({
    token,
    isLoading: true,
    errorMessage: null,
  });
  const [refreshVersion, setRefreshVersion] = useState(0);
  const ticket = loadedTicket?.token === token ? loadedTicket.ticket : null;
  const ticketStatus = ticket?.status;
  const isLoading = requestState.token !== token || requestState.isLoading;
  const errorMessage =
    requestState.token === token ? requestState.errorMessage : null;

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    async function loadTicket() {
      try {
        const status = await apiRequest(
          `/api/public/tickets/${encodeURIComponent(token)}`,
          decodeTicketStatus,
          { signal: controller.signal },
        );
        if (active) {
          setLoadedTicket({ token, ticket: status });
          setRequestState({ token, isLoading: false, errorMessage: null });
        }
      } catch (error: unknown) {
        if (active) {
          setRequestState({
            token,
            isLoading: false,
            errorMessage: getStatusErrorMessage(error),
          });
        }
      }
    }

    void loadTicket();

    return () => {
      active = false;
      controller.abort();
    };
  }, [refreshVersion, token]);

  useEffect(() => {
    if (
      ticketStatus &&
      ["SEATED", "NO_SHOW", "PICKED_UP", "CANCELLED", "EXPIRED"].includes(
        ticketStatus,
      )
    ) {
      return;
    }

    const stream = new EventSource(
      `/api/public/tickets/${encodeURIComponent(token)}/events`,
    );
    const refreshTicket = () => {
      setRefreshVersion((version) => version + 1);
    };
    stream.onopen = refreshTicket;
    stream.onmessage = refreshTicket;
    return () => stream.close();
  }, [ticketStatus, token]);

  const queueTitle =
    ticket?.type === "TABLE" ? "Table waitlist" : "Pickup order";
  const headline =
    ticket?.type === "TABLE" ? "Your place in line" : "Your pickup ticket";

  return (
    <main className={styles.page} aria-busy={isLoading}>
      <div className={styles.shell}>
        {isLoading && !ticket ? (
          <section aria-label="Loading ticket" className={styles.loadingState}>
            <p role="status">Loading your ticket</p>
            <div className={styles.skeletonTitle} />
            <div className={styles.skeletonPanel} />
            <div className={styles.skeletonText} />
          </section>
        ) : ticket ? (
          <>
            <header className={styles.header}>
              <h1>{headline}</h1>
              <p>{ticket.restaurantName}</p>
            </header>

            {errorMessage && (
              <p className={styles.refreshWarning} role="status">
                {errorMessage} Your last status is still shown below.
              </p>
            )}

            <section aria-label="Ticket details" className={styles.ticketPanel}>
              <div className={styles.ticketTopline}>
                <p className={styles.queueTitle}>{queueTitle}</p>
                <span
                  aria-live="polite"
                  className={styles.statusPill}
                  data-status={ticket.status.toLowerCase()}
                >
                  {statusLabel(ticket.status)}
                </span>
              </div>

              <div className={styles.ticketNumber}>
                <span>Ticket number</span>
                <strong>{String(ticket.ticketNumber).padStart(3, "0")}</strong>
              </div>

              {ticket.type === "TABLE" && ticket.status === "WAITING" && (
                <div className={styles.position}>
                  {ticket.queuePosition === null ? (
                    <p>{ticket.positionMessage}</p>
                  ) : (
                    <>
                      <strong>{ticket.queuePosition}</strong>
                      <span>in line</span>
                    </>
                  )}
                  {ticket.partySize !== null && (
                    <span className={styles.partySize}>
                      Party of {ticket.partySize}
                    </span>
                  )}
                </div>
              )}

              <p className={styles.nextStep}>{ticket.nextStep}</p>

              {ticket.type === "PICKUP" && ticket.positionMessage && (
                <p className={styles.positionMessage}>
                  {ticket.positionMessage}
                </p>
              )}

              <TicketPushControls token={token} status={ticket.status} />

              <div className={styles.ticketFooter}>
                <p>Updated {formatUpdatedAt(ticket.updatedAt)}</p>
                <button
                  className={styles.refreshButton}
                  disabled={isLoading}
                  onClick={() => {
                    setRequestState({
                      token,
                      isLoading: true,
                      errorMessage: null,
                    });
                    setRefreshVersion((version) => version + 1);
                  }}
                  type="button"
                >
                  {isLoading ? "Refreshing…" : "Refresh status"}
                </button>
              </div>
            </section>

            <p className={styles.privacyNote}>
              This link is private. Anyone who has it can view this ticket, so
              keep it somewhere safe. Status updates appear automatically while
              this page is open.
            </p>
          </>
        ) : (
          <section className={styles.errorState} role="alert">
            <h1>Ticket unavailable</h1>
            <p>
              {errorMessage ??
                "We couldn't find this ticket. Check that you have the full link."}
            </p>
            <button
              className={styles.refreshButton}
              disabled={isLoading}
              onClick={() => {
                setRequestState({
                  token,
                  isLoading: true,
                  errorMessage: null,
                });
                setRefreshVersion((version) => version + 1);
              }}
              type="button"
            >
              {isLoading ? "Trying again…" : "Try again"}
            </button>
          </section>
        )}
      </div>
    </main>
  );
}
