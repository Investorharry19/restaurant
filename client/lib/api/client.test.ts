import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, apiRequest } from "./client";

function decodeGreeting(data: unknown): { greeting: string } {
  if (
    typeof data !== "object" ||
    data === null ||
    !("greeting" in data) ||
    typeof data.greeting !== "string"
  ) {
    throw new Error("Invalid greeting response");
  }

  return { greeting: data.greeting };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("apiRequest", () => {
  it("uses the same-origin API proxy and validates successful response data", async () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        success: true,
        message: "Request completed",
        data: { greeting: "Hello" },
      }),
    );

    await expect(
      apiRequest("/api/public/greeting", decodeGreeting),
    ).resolves.toEqual({ greeting: "Hello" });

    const [, options] = fetch.mock.calls[0];
    expect(options?.credentials).toBe("include");
    expect(options?.cache).toBe("no-store");
    expect(new Headers(options?.headers).get("X-Requested-With")).toBe(
      "XMLHttpRequest",
    );
  });

  it("preserves stable API error codes", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        {
          success: false,
          message: "Ticket is no longer active.",
          code: "TICKET_INACTIVE",
          data: null,
        },
        { status: 409 },
      ),
    );

    await expect(
      apiRequest("/api/public/greeting", decodeGreeting),
    ).rejects.toMatchObject({
      name: "ApiError",
      status: 409,
      code: "TICKET_INACTIVE",
    });
  });

  it("rejects malformed envelopes instead of trusting their data", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({ success: true, data: { greeting: "Hello" } }),
    );

    await expect(
      apiRequest("/api/public/greeting", decodeGreeting),
    ).rejects.toBeInstanceOf(ApiError);
  });
});
