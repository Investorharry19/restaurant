"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ApiError, apiRequest } from "../../../lib/api/client";
import { decodeCreatedTicket } from "../../../lib/api/tickets";
import {
  decodePublicRestaurant,
  type PublicQueueType,
  type PublicRestaurant,
} from "../../../lib/api/restaurants";
import styles from "./restaurant-landing.module.css";

type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; restaurant: PublicRestaurant };

type LoadedState = Exclude<LoadState, { kind: "loading" }>;

const queueChoices: {
  type: PublicQueueType;
  title: string;
  description: string;
  Icon: typeof TableIcon;
}[] = [
  {
    type: "TABLE",
    title: "Join the table waitlist",
    description: "Get in line for a table and keep your place nearby.",
    Icon: TableIcon,
  },
  {
    type: "PICKUP",
    title: "Track a pickup order",
    description: "Check progress and return when your order is ready.",
    Icon: PickupIcon,
  },
];

function getLoadErrorMessage(error: unknown): string {
  if (error instanceof ApiError && error.status === 404) {
    return "This QR entry is unavailable. Check the link or ask the restaurant team for help.";
  }

  if (error instanceof ApiError) {
    return "We couldn't load this restaurant right now. Try again or ask the restaurant team for help.";
  }

  if (error instanceof Error) {
    return "We couldn't load this restaurant right now. Check your connection and try again.";
  }

  return "We couldn't load this restaurant right now. Try again or ask the restaurant team for help.";
}

function getTicketErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "PICKUP_CODE_INVALID":
        return "We couldn't find an active order for that code. Check it with the restaurant team and try again.";
      case "PICKUP_TICKET_EXISTS":
        return "This order already has a ticket. Ask the restaurant team to confirm it or issue a new pickup code.";
      case "QUEUE_CLOSED":
      case "QUEUE_UNAVAILABLE":
        return "This queue isn't accepting tickets right now. Please ask the restaurant team for help.";
      case "QUEUE_RATE_LIMITED":
        return "Too many attempts. Wait a moment, then try again.";
      case "TICKET_INVALID":
        return "Enter your name and valid party details or pickup code.";
      default:
        return "We couldn't create your ticket right now. Check your details and try again.";
    }
  }

  return "We couldn't create your ticket right now. Check your connection and try again.";
}

export function RestaurantLanding({ slug }: { slug: string }) {
  const router = useRouter();
  const [loadedState, setLoadedState] = useState<{
    key: string;
    state: LoadedState;
  } | null>(null);
  const [selection, setSelection] = useState<{
    slug: string;
    type: PublicQueueType;
  } | null>(null);
  const [ticketError, setTicketError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [retryCount, setRetryCount] = useState(0);
  const requestKey = `${slug}:${retryCount}`;
  const selectedQueue = selection?.slug === slug ? selection.type : null;
  const loadState: LoadState =
    loadedState?.key === requestKey ? loadedState.state : { kind: "loading" };

  useEffect(() => {
    let active = true;

    async function loadRestaurant() {
      try {
        const restaurant = await apiRequest(
          `/api/public/restaurants/${encodeURIComponent(slug)}`,
          decodePublicRestaurant,
        );
        if (active) {
          setLoadedState({
            key: requestKey,
            state: { kind: "ready", restaurant },
          });
        }
      } catch (error: unknown) {
        if (active) {
          setLoadedState({
            key: requestKey,
            state: { kind: "error", message: getLoadErrorMessage(error) },
          });
        }
      }
    }

    void loadRestaurant();

    return () => {
      active = false;
    };
  }, [requestKey, slug]);

  if (loadState.kind === "loading") {
    return <RestaurantLandingLoading />;
  }

  if (loadState.kind === "error") {
    return (
      <main className={styles.page}>
        <div className={styles.shell}>
          <section className={styles.messageState} role="alert">
            <h1>Restaurant details unavailable</h1>
            <p>{loadState.message}</p>
            <button
              className={styles.secondaryButton}
              onClick={() => {
                setSelection(null);
                setRetryCount((count) => count + 1);
              }}
              type="button"
            >
              Try again
            </button>
          </section>
        </div>
      </main>
    );
  }

  const { restaurant } = loadState;
  const hasEnabledQueues = restaurant.enabledQueueTypes.length > 0;
  const selectedTitle = queueChoices.find(
    (choice) => choice.type === selectedQueue,
  )?.title;

  async function submitTicket(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedQueue || isSubmitting) {
      return;
    }

    const formData = new FormData(event.currentTarget);
    const body: {
      type: PublicQueueType;
      displayName: string;
      partySize?: number;
      pickupCode?: string;
    } = {
      type: selectedQueue,
      displayName: String(formData.get("displayName") ?? "").trim(),
    };

    if (selectedQueue === "TABLE") {
      body.partySize = Number(formData.get("partySize"));
    } else {
      body.pickupCode = String(formData.get("pickupCode") ?? "")
        .trim()
        .toUpperCase();
    }

    setTicketError(null);
    setIsSubmitting(true);
    try {
      const ticket = await apiRequest(
        `/api/public/restaurants/${encodeURIComponent(slug)}/queue-tickets`,
        decodeCreatedTicket,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        },
      );
      router.push(`/t/${encodeURIComponent(ticket.publicToken)}`);
    } catch (error: unknown) {
      setTicketError(getTicketErrorMessage(error));
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <header className={styles.restaurantHeader}>
          <h1>{restaurant.name}</h1>
          {restaurant.description && (
            <p className={styles.restaurantDescription}>
              {restaurant.description}
            </p>
          )}
        </header>

        <section className={styles.choiceSection}>
          <h2 id="queue-choice-title">What would you like to do?</h2>
          <p className={styles.intro}>Choose a service to continue.</p>

          {!restaurant.isActive || !hasEnabledQueues ? (
            <p className={styles.emptyState} role="status">
              Online queues are unavailable right now. Please ask the restaurant
              team for help.
            </p>
          ) : (
            <div
              aria-labelledby="queue-choice-title"
              className={styles.routeList}
              role="group"
            >
              {queueChoices.map(({ type, title, description, Icon }) => {
                const enabled = restaurant.enabledQueueTypes.includes(type);
                const selected = selectedQueue === type;

                return (
                  <button
                    aria-pressed={selected}
                    className={styles.routeChoice}
                    disabled={!enabled || isSubmitting}
                    key={type}
                    onClick={() => {
                      setSelection({ slug, type });
                      setTicketError(null);
                    }}
                    type="button"
                  >
                    <span
                      className={`${styles.routeNode} ${selected ? styles.routeNodeSelected : ""}`}
                      aria-hidden="true"
                    >
                      {selected && <CheckIcon />}
                    </span>
                    <Icon className={styles.serviceIcon} aria-hidden="true" />
                    <span className={styles.choiceText}>
                      <span className={styles.choiceTitle}>{title}</span>
                      <span className={styles.choiceDescription}>
                        {description}
                      </span>
                    </span>
                    <span
                      className={
                        enabled ? styles.choiceAction : styles.choiceUnavailable
                      }
                    >
                      {selected
                        ? "Selected"
                        : enabled
                          ? "Choose"
                          : "Unavailable"}
                    </span>
                  </button>
                );
              })}
            </div>
          )}

          {selectedTitle && (
            <p className={styles.selectionStatus} role="status">
              {selectedTitle} selected.
            </p>
          )}

          {selectedQueue && (
            <section
              aria-labelledby="ticket-form-title"
              className={styles.ticketFormSection}
            >
              <div className={styles.ticketFormHeading}>
                <h3 id="ticket-form-title">
                  {selectedQueue === "TABLE"
                    ? "Join the table waitlist"
                    : "Find your pickup order"}
                </h3>
                <p>
                  {selectedQueue === "TABLE"
                    ? "Tell us who to look for and how many are in your party."
                    : "Enter the code the restaurant team gave you."}
                </p>
              </div>
              <form
                className={styles.ticketForm}
                key={`${slug}:${selectedQueue}`}
                onSubmit={submitTicket}
              >
                <label className={styles.formField}>
                  <span>Your name</span>
                  <input
                    autoComplete="name"
                    maxLength={120}
                    name="displayName"
                    required
                  />
                </label>

                {selectedQueue === "TABLE" ? (
                  <label className={styles.formField}>
                    <span>Party size</span>
                    <input
                      inputMode="numeric"
                      max={20}
                      min={1}
                      name="partySize"
                      required
                      type="number"
                    />
                  </label>
                ) : (
                  <label className={styles.formField}>
                    <span>Pickup code</span>
                    <input
                      aria-describedby="pickup-code-help"
                      autoCapitalize="characters"
                      autoComplete="off"
                      maxLength={11}
                      name="pickupCode"
                      placeholder="e.g. 7K2M-P9QX"
                      required
                      spellCheck={false}
                      type="text"
                    />
                    <span className={styles.formHint} id="pickup-code-help">
                      Use the 8-character code from the restaurant team.
                    </span>
                  </label>
                )}

                {ticketError && (
                  <p className={styles.formError} role="alert">
                    {ticketError}
                  </p>
                )}

                <div className={styles.formActions}>
                  <button
                    className={styles.primaryButton}
                    disabled={isSubmitting}
                    type="submit"
                  >
                    {isSubmitting ? "Creating ticket…" : "Get my ticket"}
                  </button>
                  <button
                    className={styles.textButton}
                    disabled={isSubmitting}
                    onClick={() => {
                      setSelection(null);
                      setTicketError(null);
                    }}
                    type="button"
                  >
                    Choose a different service
                  </button>
                </div>
              </form>
            </section>
          )}
        </section>

        <footer className={styles.privacyNote}>
          <PrivacyIcon aria-hidden="true" />
          <p>
            No account is needed. Your name and party size help the restaurant
            manage its table waitlist; pickup tickets are linked to your order
            code. Notifications aren&apos;t available yet, so save your ticket
            link and refresh it for updates.
          </p>
        </footer>
      </div>
    </main>
  );
}

export function RestaurantLandingLoading() {
  return (
    <main className={styles.page} aria-busy="true">
      <div className={styles.shell}>
        <p className={styles.loadingLabel} role="status">
          Loading restaurant details
        </p>
        <div className={styles.skeletonTitle} />
        <div className={styles.skeletonDescription} />
        <div className={styles.skeletonHeading} />
        <div className={styles.skeletonChoice} />
        <div className={styles.skeletonChoice} />
      </div>
    </main>
  );
}

function TableIcon({ className, ...props }: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      className={className}
      fill="none"
      height="24"
      viewBox="0 0 24 24"
      width="24"
      {...props}
    >
      <path d="M4 10h16M6 10v9m12-9v9M3 19h18M7 6h10l2 4H5l2-4Z" />
      <path d="M9 6V4h6v2" />
    </svg>
  );
}

function PickupIcon({ className, ...props }: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      className={className}
      fill="none"
      height="24"
      viewBox="0 0 24 24"
      width="24"
      {...props}
    >
      <path d="M4 8.5 12 4l8 4.5v10L12 23l-8-4.5v-10Z" />
      <path d="m4.5 8.8 7.5 4.3 7.5-4.3M12 13v9.5M9 5.7l8 4.6" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="12"
      viewBox="0 0 12 12"
      width="12"
    >
      <path d="m2 6 2.5 2.5L10 3" />
    </svg>
  );
}

function PrivacyIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg fill="none" height="20" viewBox="0 0 20 20" width="20" {...props}>
      <rect height="9" rx="1.5" width="12" x="4" y="9" />
      <path d="M6.5 9V6.5a3.5 3.5 0 0 1 7 0V9M10 12.5v2" />
    </svg>
  );
}
