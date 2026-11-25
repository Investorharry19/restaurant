import { describe, expect, it } from "vitest";
import {
  decodeWorkspaceAnalytics,
  decodeWorkspaceNotifications,
  decodeWorkspaceOrders,
} from "./workspace";

describe("workspace API decoders", () => {
  it("decodes order and notification statuses without accepting unknown values", () => {
    expect(
      decodeWorkspaceOrders([
        {
          id: "order-1",
          orderNumber: 14,
          status: "PREPARING",
          notes: null,
          createdAt: "2026-01-01T12:00:00Z",
        },
      ]),
    ).toMatchObject([{ status: "PREPARING", orderNumber: 14 }]);

    expect(() =>
      decodeWorkspaceOrders([
        {
          id: "order-1",
          orderNumber: 14,
          status: "UNKNOWN",
          notes: null,
          createdAt: "2026-01-01T12:00:00Z",
        },
      ]),
    ).toThrow("The restaurant orders response is invalid.");

    expect(() =>
      decodeWorkspaceNotifications([
        {
          id: "notification-1",
          type: "ORDER_UPDATE",
          title: "Order update",
          body: "Your order is ready.",
          status: "UNKNOWN",
          createdAt: "2026-01-01T12:00:00Z",
          sentAt: null,
        },
      ]),
    ).toThrow("The notification history response is invalid.");
  });

  it("validates the expanded operations analytics response", () => {
    const analytics = {
      tableTicketsToday: 8,
      waitingNow: 2,
      arrivedNow: 1,
      averageWaitMinutes: 12.5,
      averageCallToArrivalMinutes: null,
      averageArrivalToSeatingMinutes: 3,
      tablePartiesSeated: 4,
      tableGuestsSeated: 11,
      tableCancellations: 1,
      tableNoShows: 0,
      ordersToday: 5,
      ordersPreparing: 1,
      ordersReady: 2,
      ordersPickedUp: 3,
      orderCancellations: 1,
      averagePreparationMinutes: 9,
      averageReadyToPickupMinutes: null,
      activeQueueSize: 5,
      customersServed: 14,
      cancellations: 2,
      noShows: 0,
    };

    expect(decodeWorkspaceAnalytics(analytics)).toEqual(analytics);
    expect(() =>
      decodeWorkspaceAnalytics({
        ...analytics,
        averagePreparationMinutes: -1,
      }),
    ).toThrow("The restaurant analytics response is invalid.");
  });
});
