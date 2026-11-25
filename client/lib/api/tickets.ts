export type QueueType = "TABLE" | "PICKUP";

export type TicketStatus =
  | "WAITING"
  | "CALLED"
  | "ARRIVED"
  | "SEATED"
  | "NO_SHOW"
  | "PREPARING"
  | "READY"
  | "PICKED_UP"
  | "CANCELLED"
  | "EXPIRED";

export type CreatedTicket = {
  publicToken: string;
  ticketNumber: number;
  type: QueueType;
  status: TicketStatus;
  expiresAt: string;
};

export type TicketStatusView = {
  restaurantName: string;
  ticketNumber: number;
  type: QueueType;
  status: TicketStatus;
  partySize: number | null;
  queuePosition: number | null;
  positionMessage: string;
  nextStep: string;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isQueueType(value: unknown): value is QueueType {
  return value === "TABLE" || value === "PICKUP";
}

function isTicketStatus(value: unknown): value is TicketStatus {
  return (
    value === "WAITING" ||
    value === "CALLED" ||
    value === "ARRIVED" ||
    value === "SEATED" ||
    value === "NO_SHOW" ||
    value === "PREPARING" ||
    value === "READY" ||
    value === "PICKED_UP" ||
    value === "CANCELLED" ||
    value === "EXPIRED"
  );
}

function isCompatibleStatus(type: QueueType, status: TicketStatus): boolean {
  if (type === "TABLE") {
    return (
      status !== "PREPARING" && status !== "READY" && status !== "PICKED_UP"
    );
  }
  return (
    status !== "CALLED" &&
    status !== "ARRIVED" &&
    status !== "SEATED" &&
    status !== "NO_SHOW"
  );
}

function isDateString(value: unknown): value is string {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

function isPositiveSafeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) > 0;
}

function isNullablePositiveSafeInteger(value: unknown): value is number | null {
  return value === null || isPositiveSafeInteger(value);
}

export function decodeCreatedTicket(data: unknown): CreatedTicket {
  if (
    !isRecord(data) ||
    typeof data.publicToken !== "string" ||
    !/^[A-Za-z0-9_-]{43}$/.test(data.publicToken) ||
    !isPositiveSafeInteger(data.ticketNumber) ||
    !isQueueType(data.type) ||
    !isTicketStatus(data.status) ||
    !isCompatibleStatus(data.type, data.status) ||
    !isDateString(data.expiresAt)
  ) {
    throw new TypeError("The created ticket response is invalid.");
  }

  return {
    publicToken: data.publicToken,
    ticketNumber: data.ticketNumber,
    type: data.type,
    status: data.status,
    expiresAt: data.expiresAt,
  };
}

export function decodeTicketStatus(data: unknown): TicketStatusView {
  if (
    !isRecord(data) ||
    typeof data.restaurantName !== "string" ||
    data.restaurantName.trim() === "" ||
    !isPositiveSafeInteger(data.ticketNumber) ||
    !isQueueType(data.type) ||
    !isTicketStatus(data.status) ||
    !isNullablePositiveSafeInteger(data.partySize) ||
    !isNullablePositiveSafeInteger(data.queuePosition) ||
    typeof data.positionMessage !== "string" ||
    typeof data.nextStep !== "string" ||
    !isDateString(data.createdAt) ||
    !isDateString(data.updatedAt) ||
    !isDateString(data.expiresAt) ||
    !isCompatibleStatus(data.type, data.status) ||
    (data.type === "TABLE" &&
      (data.partySize === null || data.partySize > 20)) ||
    (data.type === "PICKUP" &&
      (data.partySize !== null || data.queuePosition !== null))
  ) {
    throw new TypeError("The ticket status response is invalid.");
  }

  return {
    restaurantName: data.restaurantName,
    ticketNumber: data.ticketNumber,
    type: data.type,
    status: data.status,
    partySize: data.partySize,
    queuePosition: data.queuePosition,
    positionMessage: data.positionMessage,
    nextStep: data.nextStep,
    createdAt: data.createdAt,
    updatedAt: data.updatedAt,
    expiresAt: data.expiresAt,
  };
}
