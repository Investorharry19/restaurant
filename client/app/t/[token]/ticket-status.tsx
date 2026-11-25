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
