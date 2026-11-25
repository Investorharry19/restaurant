import { describe, expect, it } from "vitest";
import {
  decodeBusinessApplications,
  decodeCreatedApplication,
  decodeOwnerAccountSession,
  decodePublicApplicationStatus,
} from "./applications";

describe("application API decoders", () => {
  it("accepts a created application status token", () => {
    expect(decodeCreatedApplication({ statusToken: "a".repeat(43) })).toEqual({
      statusToken: "a".repeat(43),
    });
  });

  it("decodes a verified owner session and rejects incomplete identities", () => {
    expect(
      decodeOwnerAccountSession({
        name: "Alex Owner",
        email: "alex@example.com",
      }),
    ).toEqual({ name: "Alex Owner", email: "alex@example.com" });
    expect(() =>
      decodeOwnerAccountSession({ name: "", email: "alex@example.com" }),
    ).toThrow("The owner account response is invalid.");
  });

  it("rejects malformed or incomplete application responses", () => {
    expect(() => decodeCreatedApplication({ statusToken: "short" })).toThrow(
      "The application response is invalid.",
    );
    expect(() =>
      decodePublicApplicationStatus({
        businessName: "Sample Restaurant",
        status: "APPROVED",
        restaurantSlug: null,
        reviewNote: null,
        createdAt: "2026-01-01T00:00:00Z",
        reviewedAt: null,
      }),
    ).toThrow("The application status response is invalid.");
    expect(() => decodeBusinessApplications([{}])).toThrow(
      "The application list response is invalid.",
    );
  });

  it("decodes a pending application and a pending review record", () => {
    const createdAt = "2026-01-01T00:00:00Z";
    expect(
      decodePublicApplicationStatus({
        businessName: "Sample Restaurant",
        status: "PENDING",
        restaurantSlug: null,
        reviewNote: null,
        createdAt,
        reviewedAt: null,
      }),
    ).toMatchObject({
      businessName: "Sample Restaurant",
      status: "PENDING",
      restaurantSlug: null,
    });

    expect(
      decodeBusinessApplications([
        {
          id: "00000000-0000-4000-8000-000000000001",
          businessName: "Sample Restaurant",
          businessType: "RESTAURANT",
          slug: "sample-restaurant",
          address: null,
          phone: null,
          ownerName: "Alex Owner",
          ownerEmail: "alex@example.com",
          tableQueueEnabled: true,
          pickupQueueEnabled: false,
          status: "PENDING",
          reviewNote: null,
          createdAt,
          reviewedAt: null,
        },
      ]),
    ).toHaveLength(1);
  });
});
