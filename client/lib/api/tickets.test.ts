import { describe, expect, it } from "vitest";
import { decodeCreatedTicket, decodeTicketStatus } from "./tickets";

describe("decodeCreatedTicket", () => {
  it("accepts a URL-safe 256-bit ticket token", () => {
    expect(
      decodeCreatedTicket({
        publicToken: "a".repeat(43),
        ticketNumber: 14,
        type: "TABLE",
        status: "WAITING",
        expiresAt: "2025-01-01T12:00:00Z",
      }),
    ).toEqual({
      publicToken: "a".repeat(43),
      ticketNumber: 14,
      type: "TABLE",
      status: "WAITING",
      expiresAt: "2025-01-01T12:00:00Z",
    });
  });

  it("rejects malformed or incomplete responses", () => {
    expect(() =>
      decodeCreatedTicket({
        publicToken: "not-a-token",
        ticketNumber: 14,
        type: "TABLE",
        status: "WAITING",
        expiresAt: "2025-01-01T12:00:00Z",
      }),
    ).toThrow("The created ticket response is invalid.");
  });
});

describe("decodeTicketStatus", () => {
  const validTicket = {
    restaurantName: "Sample Restaurant",
    ticketNumber: 14,
    type: "TABLE",
    status: "WAITING",
    partySize: 3,
    queuePosition: 2,
    positionMessage: "",
    nextStep: "Keep this ticket handy.",
    createdAt: "2025-01-01T12:00:00Z",
    updatedAt: "2025-01-01T12:00:00Z",
    expiresAt: "2025-01-02T12:00:00Z",
  };

  it("decodes a table ticket and server-calculated position", () => {
    expect(decodeTicketStatus(validTicket)).toMatchObject({
      type: "TABLE",
      status: "WAITING",
      partySize: 3,
      queuePosition: 2,
    });
  });

  it("rejects a pickup response that exposes table-only party size", () => {
    expect(() =>
      decodeTicketStatus({
        ...validTicket,
        type: "PICKUP",
      }),
    ).toThrow("The ticket status response is invalid.");
  });

  it("rejects queue states that are not valid for a table ticket", () => {
    expect(() =>
      decodeTicketStatus({
        ...validTicket,
        status: "PREPARING",
      }),
    ).toThrow("The ticket status response is invalid.");
  });
});
