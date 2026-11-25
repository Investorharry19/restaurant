import { describe, expect, it } from "vitest";
import { decodePublicRestaurant } from "./restaurants";

describe("decodePublicRestaurant", () => {
  it("decodes a public restaurant and its enabled queue types", () => {
    expect(
      decodePublicRestaurant({
        name: "Sample Restaurant",
        description: "A sample description.",
        logoUrl: "/api/public/restaurants/sample-restaurant/logo",
        isActive: true,
        enabledQueueTypes: ["TABLE", "PICKUP"],
      }),
    ).toEqual({
      name: "Sample Restaurant",
      description: "A sample description.",
      logoUrl: "/api/public/restaurants/sample-restaurant/logo",
      isActive: true,
      enabledQueueTypes: ["TABLE", "PICKUP"],
    });
  });

  it("rejects unknown queue types and incomplete responses", () => {
    expect(() =>
      decodePublicRestaurant({
        name: "Sample Restaurant",
        description: null,
        logoUrl: null,
        isActive: true,
        enabledQueueTypes: ["DELIVERY"],
      }),
    ).toThrow("The restaurant response is invalid.");

    expect(() =>
      decodePublicRestaurant({
        name: "Sample Restaurant",
        description: null,
        logoUrl: "https://example.com/logo.png",
        isActive: true,
        enabledQueueTypes: [],
      }),
    ).toThrow("The restaurant response is invalid.");

    expect(() => decodePublicRestaurant({ name: "Sample Restaurant" })).toThrow(
      "The restaurant response is invalid.",
    );
  });
});
