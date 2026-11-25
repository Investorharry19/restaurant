import type { DataDecoder } from "./client";

export type WorkspaceOverview = {
  id: string;
  name: string;
  slug: string;
  description: string | null;
  address: string | null;
  phone: string | null;
  timezone: string;
  role: "OWNER" | "MANAGER" | "STAFF";
  guestUrl: string;
  tableQueueEnabled: boolean;
  pickupQueueEnabled: boolean;
  waitingTables: number;
  arrivedTables: number;
  openOrders: number;
  availableTables: number;
  activeStaff: number;
};

export type WorkspaceQueueTicket = {
  id: string;
  ticketNumber: number;
  displayName: string;
  phone: string | null;
  partySize: number;
  status: "WAITING" | "CALLED" | "ARRIVED";
  createdAt: string;
  calledAt: string | null;
  queuePosition: number;
  assignedTable: string | null;
};

export type WorkspaceTable = {
  id: string;
  name: string;
  capacity: number;
  isActive: boolean;
  isOccupied: boolean;
  assignmentId: string | null;
  assignedTicketId: string | null;
  ticketNumber: number | null;
  partySize: number | null;
  assignedAt: string | null;
};

export type WorkspaceOrder = {
  id: string;
  orderNumber: number;
  status:
    "PENDING" | "ACCEPTED" | "PREPARING" | "READY" | "PICKED_UP" | "CANCELLED";
  notes: string | null;
  createdAt: string;
};

export type WorkspaceStaffMember = {
  userId: string;
  name: string;
  email: string;
  role: "OWNER" | "MANAGER" | "STAFF";
};

export type WorkspaceStaffInvitation = {
  id: string;
  email: string;
  role: "MANAGER" | "STAFF";
  createdAt: string;
  expiresAt: string;
};

export type WorkspaceStaff = {
  members: WorkspaceStaffMember[];
  invitations: WorkspaceStaffInvitation[];
};

export type WorkspaceAnalytics = {
  tableTicketsToday: number;
  waitingNow: number;
  arrivedNow: number;
  averageWaitMinutes: number | null;
  averageCallToArrivalMinutes: number | null;
  averageArrivalToSeatingMinutes: number | null;
  tablePartiesSeated: number;
  tableGuestsSeated: number;
  tableCancellations: number;
  tableNoShows: number;
  ordersToday: number;
  ordersPreparing: number;
  ordersReady: number;
  ordersPickedUp: number;
  orderCancellations: number;
  averagePreparationMinutes: number | null;
  averageReadyToPickupMinutes: number | null;
  activeQueueSize: number;
  customersServed: number;
  cancellations: number;
  noShows: number;
};

export type WorkspaceNotification = {
  id: string;
  type: string;
  title: string;
  body: string;
  status: "PENDING" | "PROCESSING" | "SENT" | "FAILED" | "CANCELLED";
  createdAt: string;
  sentAt: string | null;
};

export type CreatedStaffInvitation = {
  id: string;
  email: string;
  role: "MANAGER" | "STAFF";
  inviteUrl: string;
  expiresAt: string;
};

export type StaffInvitationDetails = {
  email: string;
  restaurantName: string;
  role: "MANAGER" | "STAFF";
};

export type StaffSession = {
  userName: string;
  restaurantName: string;
  restaurantSlug: string;
  role: "MANAGER" | "STAFF" | "OWNER";
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isString(value: unknown): value is string {
  return typeof value === "string";
}

function isNullableString(value: unknown): value is string | null {
  return value === null || isString(value);
}

function isDate(value: unknown): value is string {
  return isString(value) && Number.isFinite(Date.parse(value));
}

function isCount(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

function isNullableDuration(value: unknown): value is number | null {
  return (
    value === null ||
    (typeof value === "number" && Number.isFinite(value) && value >= 0)
  );
}

function isRole(value: unknown): value is WorkspaceStaffMember["role"] {
  return value === "OWNER" || value === "MANAGER" || value === "STAFF";
}

function isInviteRole(value: unknown): value is "MANAGER" | "STAFF" {
  return value === "MANAGER" || value === "STAFF";
}

function isOrderStatus(value: unknown): value is WorkspaceOrder["status"] {
  return (
    value === "PENDING" ||
    value === "ACCEPTED" ||
    value === "PREPARING" ||
    value === "READY" ||
    value === "PICKED_UP" ||
    value === "CANCELLED"
  );
}

function isNotificationStatus(
  value: unknown,
): value is WorkspaceNotification["status"] {
  return (
    value === "PENDING" ||
    value === "PROCESSING" ||
    value === "SENT" ||
    value === "FAILED" ||
    value === "CANCELLED"
  );
}

function decodeWorkspaceMember(value: unknown): WorkspaceStaffMember {
  if (
    !isRecord(value) ||
    !isString(value.userId) ||
    !isString(value.name) ||
    !isString(value.email) ||
    !isRole(value.role)
  ) {
    throw new TypeError("The restaurant staff response is invalid.");
  }
  return {
    userId: value.userId,
    name: value.name,
    email: value.email,
    role: value.role,
  };
}

function decodeWorkspaceInvitation(value: unknown): WorkspaceStaffInvitation {
  if (
    !isRecord(value) ||
    !isString(value.id) ||
    !isString(value.email) ||
    !isInviteRole(value.role) ||
    !isDate(value.createdAt) ||
    !isDate(value.expiresAt)
  ) {
    throw new TypeError("The restaurant staff response is invalid.");
  }
  return {
    id: value.id,
    email: value.email,
    role: value.role,
    createdAt: value.createdAt,
    expiresAt: value.expiresAt,
  };
}

export const decodeWorkspaceOverview: DataDecoder<WorkspaceOverview> = (
  value,
) => {
  if (
    !isRecord(value) ||
    !isString(value.id) ||
    !isString(value.name) ||
    !isString(value.slug) ||
    !isNullableString(value.description) ||
    !isNullableString(value.address) ||
    !isNullableString(value.phone) ||
    !isString(value.timezone) ||
    !isRole(value.role) ||
    !isString(value.guestUrl) ||
    typeof value.tableQueueEnabled !== "boolean" ||
    typeof value.pickupQueueEnabled !== "boolean" ||
    !isCount(value.waitingTables) ||
    !isCount(value.arrivedTables) ||
    !isCount(value.openOrders) ||
    !isCount(value.availableTables) ||
    !isCount(value.activeStaff)
  ) {
    throw new TypeError("The restaurant workspace response is invalid.");
  }
  return {
    id: value.id,
    name: value.name,
    slug: value.slug,
    description: value.description,
    address: value.address,
    phone: value.phone,
    timezone: value.timezone,
    role: value.role,
    guestUrl: value.guestUrl,
    tableQueueEnabled: value.tableQueueEnabled,
    pickupQueueEnabled: value.pickupQueueEnabled,
    waitingTables: value.waitingTables,
    arrivedTables: value.arrivedTables,
    openOrders: value.openOrders,
    availableTables: value.availableTables,
    activeStaff: value.activeStaff,
  };
};

export const decodeWorkspaceQueue: DataDecoder<WorkspaceQueueTicket[]> = (
  value,
) => {
  if (!Array.isArray(value)) {
    throw new TypeError("The table queue response is invalid.");
  }
  return value.map((ticket) => {
    if (
      !isRecord(ticket) ||
      !isString(ticket.id) ||
      !isCount(ticket.ticketNumber) ||
      !isString(ticket.displayName) ||
      !isNullableString(ticket.phone) ||
      !isCount(ticket.partySize) ||
      (ticket.status !== "WAITING" &&
        ticket.status !== "CALLED" &&
        ticket.status !== "ARRIVED") ||
      !isDate(ticket.createdAt) ||
      !isNullableString(ticket.calledAt) ||
      (ticket.calledAt !== null && !isDate(ticket.calledAt)) ||
      !isCount(ticket.queuePosition) ||
      !isNullableString(ticket.assignedTable)
    ) {
      throw new TypeError("The table queue response is invalid.");
    }
    return {
      id: ticket.id,
      ticketNumber: ticket.ticketNumber,
      displayName: ticket.displayName,
      phone: ticket.phone,
      partySize: ticket.partySize,
      status: ticket.status,
      createdAt: ticket.createdAt,
      calledAt: ticket.calledAt,
      queuePosition: ticket.queuePosition,
      assignedTable: ticket.assignedTable,
    };
  });
};

export const decodeWorkspaceTables: DataDecoder<WorkspaceTable[]> = (value) => {
  if (!Array.isArray(value)) {
    throw new TypeError("The restaurant tables response is invalid.");
  }
  return value.map((table) => {
    if (
      !isRecord(table) ||
      !isString(table.id) ||
      !isString(table.name) ||
      !isCount(table.capacity) ||
      typeof table.isActive !== "boolean" ||
      typeof table.isOccupied !== "boolean" ||
      !isNullableString(table.assignmentId) ||
      !isNullableString(table.assignedTicketId) ||
      !(table.ticketNumber === null || isCount(table.ticketNumber)) ||
      !(table.partySize === null || isCount(table.partySize)) ||
      !isNullableString(table.assignedAt) ||
      (table.assignedAt !== null && !isDate(table.assignedAt))
    ) {
      throw new TypeError("The restaurant tables response is invalid.");
    }
    return {
      id: table.id,
      name: table.name,
      capacity: table.capacity,
      isActive: table.isActive,
      isOccupied: table.isOccupied,
      assignmentId: table.assignmentId,
      assignedTicketId: table.assignedTicketId,
      ticketNumber: table.ticketNumber,
      partySize: table.partySize,
      assignedAt: table.assignedAt,
    };
  });
};

export const decodeWorkspaceOrders: DataDecoder<WorkspaceOrder[]> = (value) => {
  if (!Array.isArray(value)) {
    throw new TypeError("The restaurant orders response is invalid.");
  }
  return value.map((order) => {
    if (
      !isRecord(order) ||
      !isString(order.id) ||
      !isCount(order.orderNumber) ||
      !isOrderStatus(order.status) ||
      !isNullableString(order.notes) ||
      !isDate(order.createdAt)
    ) {
      throw new TypeError("The restaurant orders response is invalid.");
    }
    return {
      id: order.id,
      orderNumber: order.orderNumber,
      status: order.status,
      notes: order.notes,
      createdAt: order.createdAt,
    };
  });
};

export const decodeWorkspaceStaff: DataDecoder<WorkspaceStaff> = (value) => {
  if (
    !isRecord(value) ||
    !Array.isArray(value.members) ||
    !Array.isArray(value.invitations)
  ) {
    throw new TypeError("The restaurant staff response is invalid.");
  }
  return {
    members: value.members.map(decodeWorkspaceMember),
    invitations: value.invitations.map(decodeWorkspaceInvitation),
  };
};

export const decodeWorkspaceAnalytics: DataDecoder<WorkspaceAnalytics> = (
  value,
) => {
  if (
    !isRecord(value) ||
    !isCount(value.tableTicketsToday) ||
    !isCount(value.waitingNow) ||
    !isCount(value.arrivedNow) ||
    !isNullableDuration(value.averageWaitMinutes) ||
    !isNullableDuration(value.averageCallToArrivalMinutes) ||
    !isNullableDuration(value.averageArrivalToSeatingMinutes) ||
    !isCount(value.tablePartiesSeated) ||
    !isCount(value.tableGuestsSeated) ||
    !isCount(value.tableCancellations) ||
    !isCount(value.tableNoShows) ||
    !isCount(value.ordersToday) ||
    !isCount(value.ordersPreparing) ||
    !isCount(value.ordersReady) ||
    !isCount(value.ordersPickedUp) ||
    !isCount(value.orderCancellations) ||
    !isNullableDuration(value.averagePreparationMinutes) ||
    !isNullableDuration(value.averageReadyToPickupMinutes) ||
    !isCount(value.activeQueueSize) ||
    !isCount(value.customersServed) ||
    !isCount(value.cancellations) ||
    !isCount(value.noShows)
  ) {
    throw new TypeError("The restaurant analytics response is invalid.");
  }
  return {
    tableTicketsToday: value.tableTicketsToday,
    waitingNow: value.waitingNow,
    arrivedNow: value.arrivedNow,
    averageWaitMinutes: value.averageWaitMinutes,
    averageCallToArrivalMinutes: value.averageCallToArrivalMinutes,
    averageArrivalToSeatingMinutes: value.averageArrivalToSeatingMinutes,
    tablePartiesSeated: value.tablePartiesSeated,
    tableGuestsSeated: value.tableGuestsSeated,
    tableCancellations: value.tableCancellations,
    tableNoShows: value.tableNoShows,
    ordersToday: value.ordersToday,
    ordersPreparing: value.ordersPreparing,
    ordersReady: value.ordersReady,
    ordersPickedUp: value.ordersPickedUp,
    orderCancellations: value.orderCancellations,
    averagePreparationMinutes: value.averagePreparationMinutes,
    averageReadyToPickupMinutes: value.averageReadyToPickupMinutes,
    activeQueueSize: value.activeQueueSize,
    customersServed: value.customersServed,
    cancellations: value.cancellations,
    noShows: value.noShows,
  };
};

export const decodeWorkspaceNotifications: DataDecoder<
  WorkspaceNotification[]
> = (value) => {
  if (!Array.isArray(value)) {
    throw new TypeError("The notification history response is invalid.");
  }
  return value.map((notification) => {
    if (
      !isRecord(notification) ||
      !isString(notification.id) ||
      !isString(notification.type) ||
      !isString(notification.title) ||
      !isString(notification.body) ||
      !isNotificationStatus(notification.status) ||
      !isDate(notification.createdAt) ||
      !isNullableString(notification.sentAt) ||
      (notification.sentAt !== null && !isDate(notification.sentAt))
    ) {
      throw new TypeError("The notification history response is invalid.");
    }
    return {
      id: notification.id,
      type: notification.type,
      title: notification.title,
      body: notification.body,
      status: notification.status,
      createdAt: notification.createdAt,
      sentAt: notification.sentAt,
    };
  });
};

export const decodeCreatedStaffInvitation: DataDecoder<
  CreatedStaffInvitation
> = (value) => {
  if (
    !isRecord(value) ||
    !isString(value.id) ||
    !isString(value.email) ||
    !isInviteRole(value.role) ||
    !isString(value.inviteUrl) ||
    !isDate(value.expiresAt)
  ) {
    throw new TypeError("The staff invitation response is invalid.");
  }
  return {
    id: value.id,
    email: value.email,
    role: value.role,
    inviteUrl: value.inviteUrl,
    expiresAt: value.expiresAt,
  };
};

export const decodeStaffInvitationDetails: DataDecoder<
  StaffInvitationDetails
> = (value) => {
  if (
    !isRecord(value) ||
    !isString(value.email) ||
    !isString(value.restaurantName) ||
    !isInviteRole(value.role)
  ) {
    throw new TypeError("The staff invitation response is invalid.");
  }
  return {
    email: value.email,
    restaurantName: value.restaurantName,
    role: value.role,
  };
};

export const decodeStaffSession: DataDecoder<StaffSession> = (value) => {
  if (
    !isRecord(value) ||
    !isString(value.userName) ||
    !isString(value.restaurantName) ||
    !isString(value.restaurantSlug) ||
    !isRole(value.role)
  ) {
    throw new TypeError("The staff session response is invalid.");
  }
  return {
    userName: value.userName,
    restaurantName: value.restaurantName,
    restaurantSlug: value.restaurantSlug,
    role: value.role,
  };
};
