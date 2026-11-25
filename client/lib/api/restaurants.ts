export type PublicQueueType = "TABLE" | "PICKUP";

export type PublicRestaurant = {
  name: string;
  description: string | null;
  isActive: boolean;
  enabledQueueTypes: PublicQueueType[];
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isPublicQueueType(value: unknown): value is PublicQueueType {
  return value === "TABLE" || value === "PICKUP";
}

export function decodePublicRestaurant(data: unknown): PublicRestaurant {
  if (
    !isRecord(data) ||
    typeof data.name !== "string" ||
    data.name.trim() === "" ||
    (typeof data.description !== "string" && data.description !== null) ||
    typeof data.isActive !== "boolean" ||
    !Array.isArray(data.enabledQueueTypes) ||
    !data.enabledQueueTypes.every(isPublicQueueType)
  ) {
    throw new TypeError("The restaurant response is invalid.");
  }

  return {
    name: data.name,
    description: data.description,
    isActive: data.isActive,
    enabledQueueTypes: data.enabledQueueTypes,
  };
}
