export type ApiSuccessEnvelope = {
  success: true;
  message: string;
  data: unknown;
};

export type ApiErrorEnvelope = {
  success: false;
  message: string;
  code: string;
  data: null;
};

export type ApiEnvelope = ApiSuccessEnvelope | ApiErrorEnvelope;

export type DataDecoder<T> = (data: unknown) => T;

export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
    public readonly code: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function isApiEnvelope(value: unknown): value is ApiEnvelope {
  if (typeof value !== "object" || value === null) {
    return false;
  }

  const envelope = value as Record<string, unknown>;

  if (
    typeof envelope.message !== "string" ||
    typeof envelope.success !== "boolean" ||
    !("data" in envelope)
  ) {
    return false;
  }

  if (envelope.success) {
    return true;
  }

  return typeof envelope.code === "string" && envelope.data === null;
}

export async function apiRequest<T>(
  path: `/api/${string}`,
  decodeData: DataDecoder<T>,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  headers.set("X-Requested-With", "XMLHttpRequest");

  const response = await fetch(path, {
    ...init,
    cache: "no-store",
    credentials: "include",
    headers,
  });

  let payload: unknown;
  try {
    payload = await response.json();
  } catch (error) {
    if (error instanceof SyntaxError) {
      throw new ApiError(
        "The server returned an invalid response.",
        response.status,
        "INVALID_RESPONSE",
      );
    }

    throw error;
  }

  if (!isApiEnvelope(payload)) {
    throw new ApiError(
      "The server returned an invalid response.",
      response.status,
      "INVALID_RESPONSE",
    );
  }

  if (!response.ok || !payload.success) {
    throw new ApiError(
      payload.message,
      response.status,
      payload.success ? `HTTP_${response.status}` : payload.code,
    );
  }

  return decodeData(payload.data);
}
