"use client";

import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import { ApiError, apiRequest } from "../../../lib/api/client";
import { decodeEmptyObject } from "../../../lib/api/applications";
import {
  decodeCreatedStaffInvitation,
  decodeWorkspaceAnalytics,
  decodeWorkspaceNotifications,
  decodeWorkspaceOrders,
  decodeWorkspaceOverview,
  decodeWorkspaceQueue,
  decodeWorkspaceStaff,
  decodeWorkspaceTables,
  type CreatedStaffInvitation,
  type WorkspaceAnalytics,
  type WorkspaceNotification,
  type WorkspaceOrder,
  type WorkspaceOverview,
  type WorkspaceQueueTicket,
  type WorkspaceStaff,
  type WorkspaceTable,
} from "../../../lib/api/workspace";
import styles from "./workspace.module.css";

type WorkspaceSection =
  | "overview"
  | "queue"
  | "orders"
  | "tables"
  | "staff"
  | "notifications"
  | "analytics"
  | "settings";

const sections: Array<{
  id: WorkspaceSection;
  label: string;
  managerOnly?: boolean;
}> = [
  { id: "overview", label: "Overview" },
  { id: "queue", label: "Table waitlist" },
  { id: "orders", label: "Pickup orders" },
  { id: "tables", label: "Tables" },
  { id: "staff", label: "Staff", managerOnly: true },
  { id: "notifications", label: "Notifications" },
  { id: "analytics", label: "Analytics" },
  { id: "settings", label: "Restaurant settings", managerOnly: true },
];

type SectionErrors = Partial<Record<WorkspaceSection, string>>;

function isWorkspaceSection(value: string): value is WorkspaceSection {
  return sections.some((section) => section.id === value);
}

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message;
  }
  return "We couldn't reach the workspace. Check your connection and try again.";
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium" }).format(
    new Date(value),
  );
}

function formatDuration(minutes: number | null): string {
  return minutes === null ? "Not enough data" : `${Math.round(minutes)} min`;
}

function orderNextStatus(status: WorkspaceOrder["status"]): string | null {
  switch (status) {
    case "PENDING":
      return "ACCEPTED";
    case "ACCEPTED":
      return "PREPARING";
    case "PREPARING":
      return "READY";
    case "READY":
      return "PICKED_UP";
    default:
      return null;
  }
}

function statusLabel(status: string): string {
  return status
    .replaceAll("_", " ")
    .toLowerCase()
    .replace(/^\w/, (letter) => letter.toUpperCase());
}

export function OwnerWorkspace({ slug }: { slug: string }) {
  const router = useRouter();
  const apiRoot: `/api/${string}` = `/api/owner/workspaces/${encodeURIComponent(slug)}`;
  const [activeSection, setActiveSection] =
    useState<WorkspaceSection>("overview");
  const [overview, setOverview] = useState<WorkspaceOverview | null>(null);
  const [queue, setQueue] = useState<WorkspaceQueueTicket[]>([]);
  const [orders, setOrders] = useState<WorkspaceOrder[]>([]);
  const [tables, setTables] = useState<WorkspaceTable[]>([]);
  const [staff, setStaff] = useState<WorkspaceStaff | null>(null);
  const [analytics, setAnalytics] = useState<WorkspaceAnalytics | null>(null);
  const [notifications, setNotifications] = useState<
    WorkspaceNotification[] | null
  >(null);
  const [sectionErrors, setSectionErrors] = useState<SectionErrors>({});
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [invite, setInvite] = useState<CreatedStaffInvitation | null>(null);
  const [newOrderCode, setNewOrderCode] = useState<string | null>(null);

  const visibleSections = useMemo(
    () =>
      sections.filter(
        (section) => !section.managerOnly || overview?.role !== "STAFF",
      ),
    [overview?.role],
  );

  const handleRequestError = useCallback(
    (error: unknown, section: WorkspaceSection) => {
      if (
        error instanceof ApiError &&
        (error.code === "OWNER_AUTH_REQUIRED" ||
          error.code === "STAFF_AUTH_REQUIRED")
      ) {
        router.replace(
          `/account?next=${encodeURIComponent(`/workspace/${slug}`)}`,
        );
        return;
      }
      setSectionErrors((current) => ({
        ...current,
        [section]: getErrorMessage(error),
      }));
    },
    [router, slug],
  );

  const loadOverview = useCallback(async () => {
    try {
      const result = await apiRequest(apiRoot, decodeWorkspaceOverview);
      setOverview(result);
      setSectionErrors((current) => ({ ...current, overview: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "overview");
    }
  }, [apiRoot, handleRequestError]);

  const loadQueue = useCallback(async () => {
    try {
      const result = await apiRequest(`${apiRoot}/queue`, decodeWorkspaceQueue);
      setQueue(result);
      setSectionErrors((current) => ({ ...current, queue: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "queue");
    }
  }, [apiRoot, handleRequestError]);

  const loadOrders = useCallback(async () => {
    try {
      const result = await apiRequest(
        `${apiRoot}/orders`,
        decodeWorkspaceOrders,
      );
      setOrders(result);
      setSectionErrors((current) => ({ ...current, orders: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "orders");
    }
  }, [apiRoot, handleRequestError]);

  const loadTables = useCallback(async () => {
    try {
      const result = await apiRequest(
        `${apiRoot}/tables`,
        decodeWorkspaceTables,
      );
      setTables(result);
      setSectionErrors((current) => ({ ...current, tables: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "tables");
    }
  }, [apiRoot, handleRequestError]);

  const loadStaff = useCallback(async () => {
    try {
      const result = await apiRequest(`${apiRoot}/staff`, decodeWorkspaceStaff);
      setStaff(result);
      setSectionErrors((current) => ({ ...current, staff: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "staff");
    }
  }, [apiRoot, handleRequestError]);

  const loadAnalytics = useCallback(async () => {
    try {
      const result = await apiRequest(
        `${apiRoot}/analytics`,
        decodeWorkspaceAnalytics,
      );
      setAnalytics(result);
      setSectionErrors((current) => ({ ...current, analytics: undefined }));
    } catch (error: unknown) {
      handleRequestError(error, "analytics");
    }
  }, [apiRoot, handleRequestError]);

  const loadNotifications = useCallback(async () => {
    try {
      const result = await apiRequest(
        `${apiRoot}/notifications`,
        decodeWorkspaceNotifications,
      );
      setNotifications(result);
      setSectionErrors((current) => ({
        ...current,
        notifications: undefined,
      }));
    } catch (error: unknown) {
      handleRequestError(error, "notifications");
    }
  }, [apiRoot, handleRequestError]);

  const refreshWorkspace = useCallback(
    async (quiet = false) => {
      if (quiet) {
        setIsRefreshing(true);
      } else if (!overview) {
        setIsLoading(true);
      }
      try {
        await Promise.all([
          loadOverview(),
          loadQueue(),
          loadOrders(),
          loadTables(),
          loadStaff(),
          loadAnalytics(),
          loadNotifications(),
        ]);
      } finally {
        setIsLoading(false);
        setIsRefreshing(false);
      }
    },
    [
      loadAnalytics,
      loadNotifications,
      loadOrders,
      loadOverview,
      loadQueue,
      loadStaff,
      loadTables,
      overview,
    ],
  );

  useEffect(() => {
    const section = window.location.hash.slice(1);
    if (isWorkspaceSection(section)) {
      const frame = window.requestAnimationFrame(() => {
        setActiveSection(section);
      });
      return () => window.cancelAnimationFrame(frame);
    }
    return undefined;
  }, []);

  useEffect(() => {
    let active = true;
    const initialLoad = window.setTimeout(() => {
      void Promise.all([
        loadOverview(),
        loadQueue(),
        loadOrders(),
        loadTables(),
        loadStaff(),
        loadAnalytics(),
        loadNotifications(),
      ]).finally(() => {
        if (active) {
          setIsLoading(false);
        }
      });
    }, 0);
    const interval = window.setInterval(() => {
      void Promise.all([loadOverview(), loadQueue(), loadOrders()]);
    }, 30_000);
    return () => {
      active = false;
      window.clearTimeout(initialLoad);
      window.clearInterval(interval);
    };
  }, [
    loadAnalytics,
    loadNotifications,
    loadOrders,
    loadOverview,
    loadQueue,
    loadStaff,
    loadTables,
  ]);

  useEffect(() => {
    let refreshTimer: number | undefined;
    const stream = new EventSource(`${apiRoot}/events`, {
      withCredentials: true,
    });
    const refreshLiveState = () => {
      if (refreshTimer !== undefined) {
        window.clearTimeout(refreshTimer);
      }
      refreshTimer = window.setTimeout(() => {
        void Promise.all([
          loadOverview(),
          loadQueue(),
          loadOrders(),
          loadTables(),
          loadAnalytics(),
        ]);
      }, 100);
    };
    stream.onopen = refreshLiveState;
    stream.onmessage = refreshLiveState;
    stream.addEventListener("session.expired", () => stream.close());
    return () => {
      stream.close();
      if (refreshTimer !== undefined) {
        window.clearTimeout(refreshTimer);
      }
    };
  }, [apiRoot, loadAnalytics, loadOrders, loadOverview, loadQueue, loadTables]);

  async function runAction(
    action: () => Promise<void>,
    successMessage: string,
  ) {
    setActionError(null);
    setActionMessage(null);
    setIsSaving(true);
    try {
      await action();
      setActionMessage(successMessage);
    } catch (error: unknown) {
      setActionError(getErrorMessage(error));
    } finally {
      setIsSaving(false);
    }
  }

  async function changeTicketStatus(
    ticket: WorkspaceQueueTicket,
    status: string,
  ) {
    await runAction(
      async () => {
        await apiRequest(
          `${apiRoot}/queue/${encodeURIComponent(ticket.id)}`,
          decodeStatusAcknowledgement,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ status }),
          },
        );
        await Promise.all([loadQueue(), loadOverview()]);
      },
      `Ticket ${ticket.ticketNumber} marked ${statusLabel(status).toLowerCase()}.`,
    );
  }

  async function callNextTableTicket() {
    await runAction(async () => {
      await apiRequest(
        `${apiRoot}/queue/call-next`,
        decodeStatusAcknowledgement,
        { method: "POST" },
      );
      await Promise.all([loadQueue(), loadOverview()]);
    }, "The next waiting guest has been called.");
  }

  async function changeOrderStatus(order: WorkspaceOrder, status: string) {
    await runAction(
      async () => {
        await apiRequest(
          `${apiRoot}/orders/${encodeURIComponent(order.id)}/status`,
          decodeStatusAcknowledgement,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ status }),
          },
        );
        await Promise.all([loadOrders(), loadOverview()]);
      },
      `Order #${order.orderNumber} marked ${statusLabel(status).toLowerCase()}.`,
    );
  }

  async function assignTable(table: WorkspaceTable, ticketId: string) {
    await runAction(async () => {
      await apiRequest(
        `${apiRoot}/tables/${encodeURIComponent(table.id)}/assignments`,
        decodeAssignmentAcknowledgement,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ticketId }),
        },
      );
      await Promise.all([loadQueue(), loadTables(), loadOverview()]);
    }, `${table.name} assigned.`);
  }

  async function releaseTable(table: WorkspaceTable) {
    if (!table.assignmentId) {
      return;
    }
    if (!window.confirm(`Release ${table.name} for the next guest?`)) {
      return;
    }
    await runAction(async () => {
      const assignment = tables.find((candidate) => candidate.id === table.id);
      if (!assignment?.assignmentId) {
        throw new Error("This table no longer has an active assignment.");
      }
      await apiRequest(
        `${apiRoot}/tables/${encodeURIComponent(table.id)}/assignments/${encodeURIComponent(assignment.assignmentId)}`,
        decodeEmptyObject,
        { method: "DELETE" },
      );
      await Promise.all([loadTables(), loadOverview()]);
    }, `${table.name} is available.`);
  }

  async function createStaffInvitation(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const formData = new FormData(form);
    await runAction(async () => {
      const created = await apiRequest(
        `${apiRoot}/staff/invitations`,
        decodeCreatedStaffInvitation,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            email: String(formData.get("email") ?? ""),
            role: String(formData.get("role") ?? "STAFF"),
          }),
        },
      );
      setInvite(created);
      form.reset();
      await loadStaff();
    }, "Invitation sent. It expires in 72 hours.");
  }

  async function createPickupOrder(notes: string) {
    await runAction(async () => {
      const order = await apiRequest(`${apiRoot}/orders`, decodeCreatedOrder, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ notes }),
      });
      setNewOrderCode(order.pickupCode);
      await Promise.all([loadOrders(), loadOverview()]);
    }, "Pickup order created. Share its one-time code with the guest.");
  }

  async function createRestaurantTable(name: string, capacity: number) {
    await runAction(async () => {
      await apiRequest(`${apiRoot}/tables`, decodeCreatedTable, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, capacity }),
      });
      await Promise.all([loadTables(), loadOverview()]);
    }, "Table added.");
  }

  async function toggleTable(table: WorkspaceTable) {
    await runAction(
      async () => {
        await apiRequest(
          `${apiRoot}/tables/${encodeURIComponent(table.id)}`,
          decodeEmptyObject,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              name: table.name,
              capacity: table.capacity,
              isActive: !table.isActive,
            }),
          },
        );
        await Promise.all([loadTables(), loadOverview()]);
      },
      table.isActive ? "Table marked inactive." : "Table made available.",
    );
  }

  async function deactivateStaff(userID: string) {
    if (!window.confirm("Remove this person's access to the restaurant?")) {
      return;
    }
    await runAction(async () => {
      await apiRequest(
        `${apiRoot}/staff/${encodeURIComponent(userID)}`,
        decodeEmptyObject,
        { method: "DELETE" },
      );
      await Promise.all([loadStaff(), loadOverview()]);
    }, "Staff access removed.");
  }

  async function revokeInvitation(invitationID: string) {
    await runAction(async () => {
      await apiRequest(
        `${apiRoot}/staff/invitations/${encodeURIComponent(invitationID)}`,
        decodeEmptyObject,
        { method: "DELETE" },
      );
      await loadStaff();
    }, "Invitation revoked.");
  }

  async function copyGuestURL() {
    if (!overview) {
      return;
    }
    try {
      await navigator.clipboard.writeText(overview.guestUrl);
      setActionError(null);
      setActionMessage("Guest page link copied.");
    } catch {
      setActionError(
        "Clipboard access is unavailable. Copy the guest page URL directly.",
      );
    }
  }

  async function signOut() {
    await runAction(async () => {
      await Promise.all([
        apiRequest("/api/owner/auth/logout", decodeEmptyObject, {
          method: "POST",
        }),
        apiRequest("/api/auth/logout", decodeEmptyObject, {
          method: "POST",
        }),
      ]);
      router.replace("/account");
    }, "Signed out.");
  }

  function navigate(section: WorkspaceSection) {
    setActiveSection(section);
    window.history.replaceState(null, "", `#${section}`);
    setActionError(null);
    setActionMessage(null);
  }

  const sectionError = sectionErrors[activeSection];

  return (
    <div className={styles.page}>
      <a className={styles.skipLink} href="#workspace-main">
        Skip to workspace
      </a>
      <div className={styles.shell}>
        <aside className={styles.sidebar}>
          <Link className={styles.wordmark} href="/">
            Restaurant queues
          </Link>
          <div className={styles.restaurantIdentity}>
            <span className={styles.activeDot} aria-hidden="true" />
            <div>
              <strong>{overview?.name ?? "Restaurant workspace"}</strong>
              <span>
                {overview ? `/${overview.slug}` : "Loading restaurant…"}
              </span>
            </div>
          </div>
          <nav aria-label="Workspace sections" className={styles.navigation}>
            {visibleSections.map((section) => (
              <a
                aria-current={activeSection === section.id ? "page" : undefined}
                className={
                  activeSection === section.id
                    ? `${styles.navigationLink} ${styles.activeNavigationLink}`
                    : styles.navigationLink
                }
                href={`#${section.id}`}
                key={section.id}
                onClick={(event) => {
                  event.preventDefault();
                  navigate(section.id);
                }}
              >
                {section.label}
                {section.id === "queue" &&
                  overview &&
                  overview.waitingTables > 0 && (
                    <span className={styles.navigationCount}>
                      {overview.waitingTables}
                    </span>
                  )}
              </a>
            ))}
          </nav>
          <div className={styles.sidebarFooter}>
            <span>
              {overview?.role ? statusLabel(overview.role) : "Owner access"}
            </span>
            <button
              className={styles.textButton}
              disabled={isSaving}
              onClick={() => void signOut()}
              type="button"
            >
              Sign out
            </button>
          </div>
        </aside>

        <div className={styles.mainColumn}>
          <header className={styles.topbar}>
            <div>
              <span className={styles.topbarLabel}>Restaurant workspace</span>
              <strong>{overview?.name ?? "Your restaurant"}</strong>
            </div>
            {overview && (
              <Link
                className={styles.guestLink}
                href={`/r/${encodeURIComponent(overview.slug)}`}
                target="_blank"
              >
                Open guest page
                <span aria-hidden="true">↗</span>
              </Link>
            )}
          </header>

          <main className={styles.content} id="workspace-main">
            {isLoading && !overview ? (
              <section
                className={styles.loadingState}
                aria-label="Loading workspace"
              >
                <div className={styles.skeletonTitle} />
                <div className={styles.skeletonLine} />
                <div className={styles.skeletonPanel} />
                <p role="status">Loading your restaurant workspace…</p>
              </section>
            ) : !overview ? (
              <section className={styles.errorState} role="alert">
                <h1>Workspace unavailable</h1>
                <p>
                  {sectionErrors.overview ??
                    "We couldn't load this restaurant."}
                </p>
                <button
                  className={styles.primaryButton}
                  onClick={() => void refreshWorkspace()}
                >
                  Try again
                </button>
                <Link href="/account">Sign in to your owner account</Link>
              </section>
            ) : (
              <>
                <div className={styles.pageHeading}>
                  <div>
                    <p className={styles.breadcrumb}>
                      Today · {overview.timezone}
                    </p>
                    <h1>{sectionTitle(activeSection)}</h1>
                    <p>{sectionDescription(activeSection, overview.name)}</p>
                  </div>
                  <button
                    className={styles.refreshButton}
                    disabled={isRefreshing || isLoading}
                    onClick={() => void refreshWorkspace(true)}
                    type="button"
                  >
                    {isRefreshing ? "Refreshing…" : "Refresh"}
                  </button>
                </div>

                {(actionError || actionMessage) && (
                  <p
                    className={
                      actionError ? styles.errorMessage : styles.successMessage
                    }
                    role={actionError ? "alert" : "status"}
                  >
                    {actionError ?? actionMessage}
                  </p>
                )}
                {sectionError && (
                  <div className={styles.sectionError} role="alert">
                    <p>{sectionError}</p>
                    <button
                      className={styles.textButton}
                      onClick={() => void refreshWorkspace(true)}
                      type="button"
                    >
                      Retry
                    </button>
                  </div>
                )}

                {activeSection === "overview" && (
                  <OverviewSection
                    overview={overview}
                    queue={queue}
                    orders={orders}
                    tables={tables}
                    isSaving={isSaving}
                    onCopyLink={() => void copyGuestURL()}
                    onNavigate={navigate}
                    onTicketAction={(ticket, status) =>
                      void changeTicketStatus(ticket, status)
                    }
                    onAssign={(table, ticketID) =>
                      void assignTable(table, ticketID)
                    }
                    onOrderAction={(order, status) =>
                      void changeOrderStatus(order, status)
                    }
                  />
                )}
                {activeSection === "queue" && (
                  <QueueSection
                    queue={queue}
                    tables={tables}
                    isSaving={isSaving}
                    onCallNext={() => void callNextTableTicket()}
                    onTicketAction={(ticket, status) =>
                      void changeTicketStatus(ticket, status)
                    }
                    onAssign={(table, ticketID) =>
                      void assignTable(table, ticketID)
                    }
                  />
                )}
                {activeSection === "orders" && (
                  <OrdersSection
                    orders={orders}
                    isSaving={isSaving}
                    newOrderCode={newOrderCode}
                    onCreateOrder={(notes) => void createPickupOrder(notes)}
                    onOrderAction={(order, status) =>
                      void changeOrderStatus(order, status)
                    }
                  />
                )}
                {activeSection === "tables" && (
                  <TablesSection
                    tables={tables}
                    isSaving={isSaving}
                    onRelease={(table) => void releaseTable(table)}
                    onAddTable={(name, capacity) =>
                      void createRestaurantTable(name, capacity)
                    }
                    onToggleTable={(table) => void toggleTable(table)}
                  />
                )}
                {activeSection === "staff" && (
                  <StaffSection
                    staff={staff}
                    invite={invite}
                    isSaving={isSaving}
                    onInvite={(event) => void createStaffInvitation(event)}
                    onDeactivate={(userID) => void deactivateStaff(userID)}
                    onRevoke={(invitationID) =>
                      void revokeInvitation(invitationID)
                    }
                  />
                )}
                {activeSection === "notifications" && (
                  <NotificationsSection notifications={notifications} />
                )}
                {activeSection === "analytics" && (
                  <AnalyticsSection analytics={analytics} />
                )}
                {activeSection === "settings" && (
                  <SettingsSection
                    apiRoot={apiRoot}
                    overview={overview}
                    isSaving={isSaving}
                    onSave={(action) =>
                      runAction(async () => {
                        await action();
                        await loadOverview();
                      }, "Restaurant settings saved.")
                    }
                  />
                )}
              </>
            )}
          </main>
        </div>
      </div>
    </div>
  );
}

function sectionTitle(section: WorkspaceSection): string {
  switch (section) {
    case "overview":
      return "Service overview";
    case "queue":
      return "Table waitlist";
    case "orders":
      return "Pickup orders";
    case "tables":
      return "Tables";
    case "staff":
      return "Staff access";
    case "notifications":
      return "Notification history";
    case "analytics":
      return "Restaurant analytics";
    case "settings":
      return "Restaurant settings";
  }
}

function sectionDescription(section: WorkspaceSection, name: string): string {
  switch (section) {
    case "overview":
      return `${name}’s active services, guest entry, and next actions.`;
    case "queue":
      return "Call the next guest, mark arrivals, and seat parties at an available table.";
    case "orders":
      return "Create pickup orders and move each order through its kitchen status.";
    case "tables":
      return "Set table capacity and see which tables are available or assigned.";
    case "staff":
      return "Invite trusted people and control their restaurant access.";
    case "notifications":
      return "Review notification attempts recorded for this restaurant.";
    case "analytics":
      return "A factual view of today’s recorded table and pickup activity.";
    case "settings":
      return "Keep your guest page and available queue services up to date.";
  }
}

function OverviewSection({
  overview,
  queue,
  orders,
  tables,
  isSaving,
  onCopyLink,
  onNavigate,
  onTicketAction,
  onAssign,
  onOrderAction,
}: {
  overview: WorkspaceOverview;
  queue: WorkspaceQueueTicket[];
  orders: WorkspaceOrder[];
  tables: WorkspaceTable[];
  isSaving: boolean;
  onCopyLink: () => void;
  onNavigate: (section: WorkspaceSection) => void;
  onTicketAction: (ticket: WorkspaceQueueTicket, status: string) => void;
  onAssign: (table: WorkspaceTable, ticketID: string) => void;
  onOrderAction: (order: WorkspaceOrder, status: string) => void;
}) {
  const activeOrders = orders.filter(
    (order) => !["PICKED_UP", "CANCELLED"].includes(order.status),
  );
  const availableTables = tables.filter(
    (table) => table.isActive && !table.isOccupied,
  );

  return (
    <div className={styles.overviewGrid}>
      <div className={styles.operationsColumn}>
        <section
          className={styles.serviceSection}
          aria-labelledby="table-service-title"
        >
          <div className={styles.sectionHeader}>
            <div>
              <h2 id="table-service-title">Table waitlist</h2>
              <p>
                {overview.tableQueueEnabled
                  ? `${overview.waitingTables} waiting · ${overview.arrivedTables} arrived`
                  : "Not currently offered to guests"}
              </p>
            </div>
            <button
              className={styles.inlineLink}
              onClick={() => onNavigate("queue")}
            >
              Open waitlist
            </button>
          </div>
          {queue.length === 0 ? (
            <p className={styles.emptyState}>
              {overview.tableQueueEnabled
                ? "No guests are waiting right now."
                : "Turn on the table waitlist in restaurant settings when you’re ready."}
            </p>
          ) : (
            <div className={styles.liveRows}>
              {queue.slice(0, 4).map((ticket) => (
                <QueueRow
                  key={ticket.id}
                  ticket={ticket}
                  tables={availableTables}
                  isSaving={isSaving}
                  onAction={onTicketAction}
                  onAssign={onAssign}
                  compact
                />
              ))}
            </div>
          )}
        </section>

        <section
          className={styles.serviceSection}
          aria-labelledby="pickup-service-title"
        >
          <div className={styles.sectionHeader}>
            <div>
              <h2 id="pickup-service-title">Pickup orders</h2>
              <p>
                {overview.pickupQueueEnabled
                  ? `${activeOrders.length} in progress`
                  : "Not currently offered to guests"}
              </p>
            </div>
            <button
              className={styles.inlineLink}
              onClick={() => onNavigate("orders")}
            >
              Open orders
            </button>
          </div>
          {activeOrders.length === 0 ? (
            <p className={styles.emptyState}>
              {overview.pickupQueueEnabled
                ? "No pickup orders are in progress."
                : "Turn on pickup in restaurant settings when you’re ready."}
            </p>
          ) : (
            <div className={styles.liveRows}>
              {activeOrders.slice(0, 4).map((order) => (
                <OrderRow
                  key={order.id}
                  order={order}
                  isSaving={isSaving}
                  onAction={onOrderAction}
                  compact
                />
              ))}
            </div>
          )}
        </section>

        <section
          className={styles.capacityStrip}
          aria-label="Restaurant capacity"
        >
          <p>
            <strong>{availableTables.length}</strong>
            <span>tables ready</span>
          </p>
          <p>
            <strong>{overview.activeStaff}</strong>
            <span>active team members</span>
          </p>
          <button
            className={styles.inlineLink}
            onClick={() => onNavigate("analytics")}
          >
            Today’s activity
          </button>
        </section>
      </div>

      <section
        className={styles.guestPanel}
        aria-labelledby="guest-entry-title"
      >
        <div className={styles.guestPanelHeading}>
          <div>
            <p className={styles.panelEyebrow}>For your guests</p>
            <h2 id="guest-entry-title">Your entry page is live</h2>
          </div>
          <span className={styles.liveBadge}>Live</span>
        </div>
        <a
          className={styles.guestPageLink}
          href={overview.guestUrl}
          rel="noreferrer"
          target="_blank"
        >
          {overview.guestUrl}
          <span aria-hidden="true">↗</span>
        </a>
        <div className={styles.qrFrame}>
          {/* This image is generated by the authenticated Go QR endpoint. */}
          <Image
            alt={`QR code linking to ${overview.name}’s guest page`}
            height={220}
            unoptimized
            src={`/api/owner/workspaces/${encodeURIComponent(overview.slug)}/qr`}
            width={220}
          />
        </div>
        <p className={styles.qrCaption}>
          Print this code for your entrance or counter. Guests can scan it to
          see the queue services you have enabled.
        </p>
        <div className={styles.guestActions}>
          <button
            className={styles.primaryButton}
            onClick={onCopyLink}
            type="button"
          >
            Copy guest link
          </button>
          <a
            className={styles.secondaryButton}
            download
            href={`/api/owner/workspaces/${encodeURIComponent(overview.slug)}/qr?download=1`}
          >
            Download QR
          </a>
        </div>
        <div className={styles.enabledServices}>
          <span>Guest services</span>
          <span
            className={
              overview.tableQueueEnabled
                ? styles.serviceEnabled
                : styles.serviceDisabled
            }
          >
            Table waitlist {overview.tableQueueEnabled ? "on" : "off"}
          </span>
          <span
            className={
              overview.pickupQueueEnabled
                ? styles.serviceEnabled
                : styles.serviceDisabled
            }
          >
            Pickup {overview.pickupQueueEnabled ? "on" : "off"}
          </span>
        </div>
      </section>
    </div>
  );
}

function QueueSection({
  queue,
  tables,
  isSaving,
  onCallNext,
  onTicketAction,
  onAssign,
}: {
  queue: WorkspaceQueueTicket[];
  tables: WorkspaceTable[];
  isSaving: boolean;
  onCallNext: () => void;
  onTicketAction: (ticket: WorkspaceQueueTicket, status: string) => void;
  onAssign: (table: WorkspaceTable, ticketID: string) => void;
}) {
  if (queue.length === 0) {
    return (
      <section className={styles.emptyPanel}>
        <h2>No guests in the table waitlist</h2>
        <p>New guests appear here after they join from your restaurant page.</p>
      </section>
    );
  }
  return (
    <>
      <div className={styles.pageHeading}>
        <p>
          {queue.some((ticket) => ticket.status === "WAITING")
            ? "Call guests in arrival order."
            : "No waiting guests to call."}
        </p>
        <button
          className={styles.primaryButton}
          disabled={
            isSaving || !queue.some((ticket) => ticket.status === "WAITING")
          }
          onClick={onCallNext}
          type="button"
        >
          Call next guest
        </button>
      </div>
      <section className={styles.board} aria-label="Current table waitlist">
        <div className={`${styles.boardHeader} ${styles.queueColumns}`}>
          <span>Guest</span>
          <span>Party</span>
          <span>Joined</span>
          <span>Status</span>
          <span>Next action</span>
        </div>
        {queue.map((ticket) => (
          <QueueRow
            key={ticket.id}
            ticket={ticket}
            tables={tables.filter(
              (table) =>
                table.isActive &&
                !table.isOccupied &&
                table.capacity >= ticket.partySize,
            )}
            isSaving={isSaving}
            onAction={onTicketAction}
            onAssign={onAssign}
          />
        ))}
      </section>
    </>
  );
}

function QueueRow({
  ticket,
  tables,
  isSaving,
  onAction,
  onAssign,
  compact = false,
}: {
  ticket: WorkspaceQueueTicket;
  tables: WorkspaceTable[];
  isSaving: boolean;
  onAction: (ticket: WorkspaceQueueTicket, status: string) => void;
  onAssign: (table: WorkspaceTable, ticketID: string) => void;
  compact?: boolean;
}) {
  return (
    <div
      className={`${styles.boardRow} ${styles.queueColumns} ${compact ? styles.compactRow : ""}`}
    >
      <div className={styles.guestCell}>
        <strong>
          #{ticket.ticketNumber} · {ticket.displayName}
        </strong>
        {ticket.phone && <span>{ticket.phone}</span>}
      </div>
      <span>
        {ticket.partySize} {ticket.partySize === 1 ? "guest" : "guests"}
      </span>
      <span>{formatTime(ticket.createdAt)}</span>
      <span className={styles.statusValue}>{statusLabel(ticket.status)}</span>
      <div className={styles.rowActions}>
        {ticket.status === "WAITING" && (
          <button
            disabled={isSaving}
            onClick={() => onAction(ticket, "CALLED")}
            type="button"
          >
            Call guest
          </button>
        )}
        {ticket.status === "CALLED" && (
          <>
            <button
              disabled={isSaving}
              onClick={() => onAction(ticket, "ARRIVED")}
              type="button"
            >
              Mark arrived
            </button>
            <button
              className={styles.subtleAction}
              disabled={isSaving}
              onClick={() => onAction(ticket, "NO_SHOW")}
              type="button"
            >
              No-show
            </button>
          </>
        )}
        {ticket.status === "ARRIVED" && (
          <>
            <select
              aria-label={`Assign a table to ${ticket.displayName}`}
              defaultValue=""
              disabled={isSaving || tables.length === 0}
              onChange={(event) => {
                const table = tables.find(
                  (candidate) => candidate.id === event.target.value,
                );
                if (table) {
                  onAssign(table, ticket.id);
                }
                event.currentTarget.value = "";
              }}
            >
              <option value="">Seat at…</option>
              {tables.map((table) => (
                <option key={table.id} value={table.id}>
                  {table.name} · up to {table.capacity}
                </option>
              ))}
            </select>
            <button
              className={styles.subtleAction}
              disabled={isSaving}
              onClick={() => onAction(ticket, "NO_SHOW")}
              type="button"
            >
              No-show
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function OrdersSection({
  orders,
  isSaving,
  newOrderCode,
  onCreateOrder,
  onOrderAction,
}: {
  orders: WorkspaceOrder[];
  isSaving: boolean;
  newOrderCode: string | null;
  onCreateOrder: (notes: string) => void;
  onOrderAction: (order: WorkspaceOrder, status: string) => void;
}) {
  function createOrder(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    onCreateOrder(String(formData.get("notes") ?? ""));
  }

  const activeOrders = orders.filter(
    (order) => !["PICKED_UP", "CANCELLED"].includes(order.status),
  );

  return (
    <div className={styles.managementLayout}>
      <section className={styles.primaryWorkArea}>
        <div className={styles.sectionHeader}>
          <div>
            <h2>Kitchen queue</h2>
            <p>Advance an order when its preparation state changes.</p>
          </div>
        </div>
        {activeOrders.length === 0 ? (
          <p className={styles.emptyState}>No pickup orders are in progress.</p>
        ) : (
          <div className={styles.liveRows}>
            {activeOrders.map((order) => (
              <OrderRow
                key={order.id}
                order={order}
                isSaving={isSaving}
                onAction={onOrderAction}
              />
            ))}
          </div>
        )}
      </section>
      <aside className={styles.actionPanel}>
        <h2>Record an order</h2>
        <p>Enter an order for pickup and give the guest their one-time code.</p>
        <form
          className={styles.form}
          onSubmit={(event) => void createOrder(event)}
        >
          <label>
            Order note <span>Optional</span>
            <textarea maxLength={2000} name="notes" rows={3} />
          </label>
          <button
            className={styles.primaryButton}
            disabled={isSaving}
            type="submit"
          >
            Create pickup order
          </button>
        </form>
        {newOrderCode && (
          <div className={styles.codeNotice} role="status">
            <strong>Share this pickup code now</strong>
            <span>{newOrderCode}</span>
            <p>
              The code is shown only once. The guest uses it on your public
              page.
            </p>
          </div>
        )}
      </aside>
    </div>
  );
}

function OrderRow({
  order,
  isSaving,
  onAction,
  compact = false,
}: {
  order: WorkspaceOrder;
  isSaving: boolean;
  onAction: (order: WorkspaceOrder, status: string) => void;
  compact?: boolean;
}) {
  const nextStatus = orderNextStatus(order.status);
  return (
    <div className={`${styles.orderRow} ${compact ? styles.compactRow : ""}`}>
      <div className={styles.orderIdentity}>
        <strong>Order #{order.orderNumber}</strong>
        <span>
          {formatTime(order.createdAt)} · {statusLabel(order.status)}
        </span>
        {order.notes && <p>{order.notes}</p>}
      </div>
      {nextStatus && (
        <button
          disabled={isSaving}
          onClick={() => onAction(order, nextStatus)}
          type="button"
        >
          {nextOrderAction(nextStatus)}
        </button>
      )}
    </div>
  );
}

function nextOrderAction(status: string): string {
  switch (status) {
    case "ACCEPTED":
      return "Accept order";
    case "PREPARING":
      return "Start preparing";
    case "READY":
      return "Mark ready";
    case "PICKED_UP":
      return "Mark picked up";
    default:
      return "Update order";
  }
}

function TablesSection({
  tables,
  isSaving,
  onRelease,
  onAddTable,
  onToggleTable,
}: {
  tables: WorkspaceTable[];
  isSaving: boolean;
  onRelease: (table: WorkspaceTable) => void;
  onAddTable: (name: string, capacity: number) => void;
  onToggleTable: (table: WorkspaceTable) => void;
}) {
  function addTable(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    onAddTable(
      String(formData.get("name") ?? ""),
      Number(formData.get("capacity") ?? 2),
    );
  }

  return (
    <div className={styles.managementLayout}>
      <section className={styles.primaryWorkArea}>
        <div className={styles.sectionHeader}>
          <div>
            <h2>Table availability</h2>
            <p>Assigned tables remain unavailable until released.</p>
          </div>
        </div>
        {tables.length === 0 ? (
          <p className={styles.emptyState}>
            Add your first table to start seating arrived guests.
          </p>
        ) : (
          <div className={styles.tableList}>
            {tables.map((table) => (
              <article className={styles.tableRow} key={table.id}>
                <div>
                  <strong>{table.name}</strong>
                  <span>Seats {table.capacity}</span>
                </div>
                <span
                  className={
                    table.isOccupied
                      ? styles.statusValue
                      : styles.availableValue
                  }
                >
                  {table.isOccupied
                    ? `In use · guest #${table.ticketNumber ?? ""}`
                    : table.isActive
                      ? "Available"
                      : "Inactive"}
                </span>
                {table.isOccupied ? (
                  <button
                    className={styles.subtleAction}
                    disabled={isSaving}
                    onClick={() => onRelease(table)}
                    type="button"
                  >
                    Release table
                  </button>
                ) : (
                  <button
                    className={styles.subtleAction}
                    disabled={isSaving}
                    onClick={() => onToggleTable(table)}
                    type="button"
                  >
                    {table.isActive ? "Make inactive" : "Make available"}
                  </button>
                )}
              </article>
            ))}
          </div>
        )}
      </section>
      <aside className={styles.actionPanel}>
        <h2>Add a table</h2>
        <p>Table names should match the signs staff and guests already use.</p>
        <form
          className={styles.form}
          onSubmit={(event) => void addTable(event)}
        >
          <label>
            Table name
            <input maxLength={80} name="name" required />
          </label>
          <label>
            Seating capacity
            <input
              defaultValue={2}
              max={30}
              min={1}
              name="capacity"
              required
              type="number"
            />
          </label>
          <button
            className={styles.primaryButton}
            disabled={isSaving}
            type="submit"
          >
            Add table
          </button>
        </form>
      </aside>
    </div>
  );
}

function StaffSection({
  staff,
  invite,
  isSaving,
  onInvite,
  onDeactivate,
  onRevoke,
}: {
  staff: WorkspaceStaff | null;
  invite: CreatedStaffInvitation | null;
  isSaving: boolean;
  onInvite: (event: FormEvent<HTMLFormElement>) => void;
  onDeactivate: (userID: string) => void;
  onRevoke: (invitationID: string) => void;
}) {
  return (
    <div className={styles.managementLayout}>
      <section className={styles.primaryWorkArea}>
        <div className={styles.sectionHeader}>
          <div>
            <h2>People with access</h2>
            <p>Only active restaurant memberships can use this workspace.</p>
          </div>
        </div>
        {!staff || staff.members.length === 0 ? (
          <p className={styles.emptyState}>
            No active staff memberships were found.
          </p>
        ) : (
          <div className={styles.tableList}>
            {staff.members.map((member) => (
              <article className={styles.staffRow} key={member.userId}>
                <div>
                  <strong>{member.name}</strong>
                  <span>{member.email}</span>
                </div>
                <span className={styles.statusValue}>
                  {statusLabel(member.role)}
                </span>
                {member.role !== "OWNER" && (
                  <button
                    className={styles.subtleAction}
                    disabled={isSaving}
                    onClick={() => onDeactivate(member.userId)}
                    type="button"
                  >
                    Remove access
                  </button>
                )}
              </article>
            ))}
          </div>
        )}
        <div className={styles.pendingInvitations}>
          <h3>Pending invitations</h3>
          {staff?.invitations.length ? (
            staff.invitations.map((pending) => (
              <div className={styles.invitationRow} key={pending.id}>
                <span>
                  {pending.email} · {statusLabel(pending.role)}
                </span>
                <span>Expires {formatDate(pending.expiresAt)}</span>
                <button
                  className={styles.textButton}
                  disabled={isSaving}
                  onClick={() => onRevoke(pending.id)}
                  type="button"
                >
                  Revoke
                </button>
              </div>
            ))
          ) : (
            <p>No pending invitations.</p>
          )}
        </div>
      </section>
      <aside className={styles.actionPanel}>
        <h2>Invite someone</h2>
        <p>They’ll receive a secure link that expires after 72 hours.</p>
        <form className={styles.form} onSubmit={onInvite}>
          <label>
            Email
            <input autoComplete="email" name="email" required type="email" />
          </label>
          <label>
            Workspace role
            <select defaultValue="STAFF" name="role">
              <option value="STAFF">Staff · daily operations</option>
              <option value="MANAGER">Manager · operations and settings</option>
            </select>
          </label>
          <button
            className={styles.primaryButton}
            disabled={isSaving}
            type="submit"
          >
            Send invitation
          </button>
        </form>
        {invite && (
          <div className={styles.codeNotice} role="status">
            <strong>Invitation sent to {invite.email}</strong>
            <a href={invite.inviteUrl}>{invite.inviteUrl}</a>
            <p>Keep this link private. It grants access to this restaurant.</p>
          </div>
        )}
      </aside>
    </div>
  );
}

function NotificationsSection({
  notifications,
}: {
  notifications: WorkspaceNotification[] | null;
}) {
  if (!notifications?.length) {
    return (
      <section className={styles.emptyPanel}>
        <h2>No notification history yet</h2>
        <p>
          This view shows delivery records when the notification service is
          enabled. It does not imply a customer received or read a message.
        </p>
      </section>
    );
  }
  return (
    <section
      className={styles.notificationList}
      aria-label="Notification history"
    >
      {notifications.map((notification) => (
        <article className={styles.notificationRow} key={notification.id}>
          <div>
            <strong>{notification.title}</strong>
            <p>{notification.body}</p>
            <span>{formatDate(notification.createdAt)}</span>
          </div>
          <span className={styles.statusValue}>
            {statusLabel(notification.status)}
          </span>
        </article>
      ))}
    </section>
  );
}

function AnalyticsSection({
  analytics,
}: {
  analytics: WorkspaceAnalytics | null;
}) {
  if (!analytics) {
    return (
      <p className={styles.emptyState}>Today’s activity is unavailable.</p>
    );
  }
  return (
    <section
      className={styles.analyticsTable}
      aria-label="Today's restaurant activity"
    >
      <div className={styles.analyticsRow}>
        <span>Table tickets created today</span>
        <strong>{analytics.tableTicketsToday}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Guests waiting now</span>
        <strong>{analytics.waitingNow}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Guests arrived</span>
        <strong>{analytics.arrivedNow}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Average time to call</span>
        <strong>{formatDuration(analytics.averageWaitMinutes)}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Average call to arrival</span>
        <strong>{formatDuration(analytics.averageCallToArrivalMinutes)}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Average arrival to seating</span>
        <strong>
          {formatDuration(analytics.averageArrivalToSeatingMinutes)}
        </strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Table parties · guests seated</span>
        <strong>
          {analytics.tablePartiesSeated} · {analytics.tableGuestsSeated}
        </strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Pickup orders recorded today</span>
        <strong>{analytics.ordersToday}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Preparing · ready</span>
        <strong>
          {analytics.ordersPreparing} · {analytics.ordersReady}
        </strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Average order preparation</span>
        <strong>{formatDuration(analytics.averagePreparationMinutes)}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Average ready to pickup</span>
        <strong>{formatDuration(analytics.averageReadyToPickupMinutes)}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Active queue size</span>
        <strong>{analytics.activeQueueSize}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Customers served</span>
        <strong>{analytics.customersServed}</strong>
      </div>
      <div className={styles.analyticsRow}>
        <span>Cancellations · no-shows</span>
        <strong>
          {analytics.cancellations} · {analytics.noShows}
        </strong>
      </div>
      <p className={styles.dataNote}>
        Counts come from recorded activity in the restaurant’s local business
        day.
      </p>
    </section>
  );
}

function SettingsSection({
  apiRoot,
  overview,
  isSaving,
  onSave,
}: {
  apiRoot: `/api/${string}`;
  overview: WorkspaceOverview;
  isSaving: boolean;
  onSave: (action: () => Promise<void>) => Promise<void>;
}) {
  function saveSettings(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formData = new FormData(event.currentTarget);
    void onSave(async () => {
      await apiRequest(apiRoot, decodeEmptyObject, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: String(formData.get("name") ?? ""),
          description: String(formData.get("description") ?? ""),
          address: String(formData.get("address") ?? ""),
          phone: String(formData.get("phone") ?? ""),
          tableQueueEnabled: formData.get("tableQueueEnabled") === "on",
          pickupQueueEnabled: formData.get("pickupQueueEnabled") === "on",
        }),
      });
    });
  }

  return (
    <section className={styles.settingsPanel}>
      <form
        className={styles.settingsForm}
        onSubmit={(event) => void saveSettings(event)}
      >
        <label>
          Restaurant name
          <input
            defaultValue={overview.name}
            maxLength={120}
            name="name"
            required
          />
        </label>
        <label>
          Guest page description
          <textarea
            defaultValue={overview.description ?? ""}
            maxLength={2000}
            name="description"
            rows={4}
          />
        </label>
        <label>
          Address
          <input
            defaultValue={overview.address ?? ""}
            maxLength={500}
            name="address"
          />
        </label>
        <label>
          Phone
          <input
            autoComplete="tel"
            defaultValue={overview.phone ?? ""}
            maxLength={50}
            name="phone"
            type="tel"
          />
        </label>
        <fieldset className={styles.serviceSettings}>
          <legend>Guest queue services</legend>
          <label>
            <input
              defaultChecked={overview.tableQueueEnabled}
              name="tableQueueEnabled"
              type="checkbox"
            />
            <span>
              <strong>Table waitlist</strong>
              <small>Let guests join a table queue from the public page.</small>
            </span>
          </label>
          <label>
            <input
              defaultChecked={overview.pickupQueueEnabled}
              name="pickupQueueEnabled"
              type="checkbox"
            />
            <span>
              <strong>Pickup orders</strong>
              <small>
                Allow guests to follow a pickup order using its code.
              </small>
            </span>
          </label>
        </fieldset>
        <div className={styles.formActions}>
          <button
            className={styles.primaryButton}
            disabled={isSaving}
            type="submit"
          >
            {isSaving ? "Saving…" : "Save restaurant settings"}
          </button>
          <span>Changes update your public guest page.</span>
        </div>
      </form>
    </section>
  );
}

function decodeStatusAcknowledgement(value: unknown): { status: string } {
  if (
    typeof value !== "object" ||
    value === null ||
    !("status" in value) ||
    typeof value.status !== "string"
  ) {
    throw new TypeError("The operation response is invalid.");
  }
  return { status: value.status };
}

function decodeAssignmentAcknowledgement(value: unknown): {
  assignmentId: string;
} {
  if (
    typeof value !== "object" ||
    value === null ||
    !("assignmentId" in value) ||
    typeof value.assignmentId !== "string"
  ) {
    throw new TypeError("The table assignment response is invalid.");
  }
  return { assignmentId: value.assignmentId };
}

function decodeCreatedOrder(value: unknown): {
  id: string;
  orderNumber: number;
  pickupCode: string;
} {
  if (
    typeof value !== "object" ||
    value === null ||
    !("id" in value) ||
    typeof value.id !== "string" ||
    !("orderNumber" in value) ||
    typeof value.orderNumber !== "number" ||
    !("pickupCode" in value) ||
    typeof value.pickupCode !== "string"
  ) {
    throw new TypeError("The created order response is invalid.");
  }
  return {
    id: value.id,
    orderNumber: value.orderNumber,
    pickupCode: value.pickupCode,
  };
}

function decodeCreatedTable(value: unknown): { id: string } {
  if (
    typeof value !== "object" ||
    value === null ||
    !("id" in value) ||
    typeof value.id !== "string"
  ) {
    throw new TypeError("The created table response is invalid.");
  }
  return { id: value.id };
}
