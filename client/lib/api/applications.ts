export type BusinessType = "RESTAURANT" | "BAKERY" | "EATERY";
export type ApplicationStatus = "PENDING" | "APPROVED" | "REJECTED";
export type ApplicationListFilter = ApplicationStatus | "ALL";

export type CreatedApplication = {
  statusToken: string;
};

export type PublicApplicationStatus = {
  businessName: string;
  status: ApplicationStatus;
  restaurantSlug: string | null;
  reviewNote: string | null;
  createdAt: string;
  reviewedAt: string | null;
};

export type BusinessApplication = {
  id: string;
  businessName: string;
  businessType: BusinessType;
  slug: string;
  address: string | null;
  phone: string | null;
  ownerName: string;
  ownerEmail: string;
  tableQueueEnabled: boolean;
  pickupQueueEnabled: boolean;
  status: ApplicationStatus;
  reviewNote: string | null;
  logoUrl: string | null;
  createdAt: string;
  reviewedAt: string | null;
};

export type OwnerApplicationDraft = {
  businessName: string;
  businessType: BusinessType;
  slug: string;
  address: string | null;
  phone: string | null;
  tableQueueEnabled: boolean;
  pickupQueueEnabled: boolean;
  logoUrl: string | null;
  reviewNote: string | null;
};

export type PlatformAdminSession = {
  name: string;
  email: string;
};

export type OwnerAccountSession = {
  name: string;
  email: string;
};

export type ApprovedApplication = {
  businessName: string;
  slug: string;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isBusinessType(value: unknown): value is BusinessType {
  return value === "RESTAURANT" || value === "BAKERY" || value === "EATERY";
}

function isApplicationStatus(value: unknown): value is ApplicationStatus {
  return value === "PENDING" || value === "APPROVED" || value === "REJECTED";
}

function isNullableString(value: unknown): value is string | null {
  return value === null || typeof value === "string";
}

function isDateString(value: unknown): value is string {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

export function decodeCreatedApplication(data: unknown): CreatedApplication {
  if (
    !isRecord(data) ||
    typeof data.statusToken !== "string" ||
    !/^[A-Za-z0-9_-]{43}$/.test(data.statusToken)
  ) {
    throw new TypeError("The application response is invalid.");
  }

  return { statusToken: data.statusToken };
}

export function decodePublicApplicationStatus(
  data: unknown,
): PublicApplicationStatus {
  if (
    !isRecord(data) ||
    typeof data.businessName !== "string" ||
    data.businessName.trim() === "" ||
    !isApplicationStatus(data.status) ||
    !isNullableString(data.restaurantSlug) ||
    !isNullableString(data.reviewNote) ||
    !isDateString(data.createdAt) ||
    (data.reviewedAt !== null && !isDateString(data.reviewedAt)) ||
    (data.status === "APPROVED" &&
      (typeof data.restaurantSlug !== "string" ||
        data.restaurantSlug.trim() === "")) ||
    (data.status !== "REJECTED" && data.reviewNote !== null)
  ) {
    throw new TypeError("The application status response is invalid.");
  }

  return {
    businessName: data.businessName,
    status: data.status,
    restaurantSlug: data.restaurantSlug,
    reviewNote: data.reviewNote,
    createdAt: data.createdAt,
    reviewedAt: data.reviewedAt,
  };
}

function decodeBusinessApplication(data: unknown): BusinessApplication {
  if (
    !isRecord(data) ||
    typeof data.id !== "string" ||
    typeof data.businessName !== "string" ||
    !isBusinessType(data.businessType) ||
    typeof data.slug !== "string" ||
    !isNullableString(data.address) ||
    !isNullableString(data.phone) ||
    typeof data.ownerName !== "string" ||
    typeof data.ownerEmail !== "string" ||
    typeof data.tableQueueEnabled !== "boolean" ||
    typeof data.pickupQueueEnabled !== "boolean" ||
    !isApplicationStatus(data.status) ||
    !isNullableString(data.reviewNote) ||
    !isNullableString(data.logoUrl) ||
    !isDateString(data.createdAt) ||
    (data.reviewedAt !== null && !isDateString(data.reviewedAt))
  ) {
    throw new TypeError("The application list response is invalid.");
  }

  return {
    id: data.id,
    businessName: data.businessName,
    businessType: data.businessType,
    slug: data.slug,
    address: data.address,
    phone: data.phone,
    ownerName: data.ownerName,
    ownerEmail: data.ownerEmail,
    tableQueueEnabled: data.tableQueueEnabled,
    pickupQueueEnabled: data.pickupQueueEnabled,
    status: data.status,
    reviewNote: data.reviewNote,
    logoUrl: data.logoUrl,
    createdAt: data.createdAt,
    reviewedAt: data.reviewedAt,
  };
}

export function decodeBusinessApplications(
  data: unknown,
): BusinessApplication[] {
  if (!Array.isArray(data)) {
    throw new TypeError("The application list response is invalid.");
  }

  return data.map(decodeBusinessApplication);
}

export function decodeOwnerApplicationDraft(
  data: unknown,
): OwnerApplicationDraft {
  if (
    !isRecord(data) ||
    typeof data.businessName !== "string" ||
    !isBusinessType(data.businessType) ||
    typeof data.slug !== "string" ||
    !isNullableString(data.address) ||
    !isNullableString(data.phone) ||
    typeof data.tableQueueEnabled !== "boolean" ||
    typeof data.pickupQueueEnabled !== "boolean" ||
    !isNullableString(data.logoUrl) ||
    !isNullableString(data.reviewNote)
  ) {
    throw new TypeError("The rejected application response is invalid.");
  }
  return {
    businessName: data.businessName,
    businessType: data.businessType,
    slug: data.slug,
    address: data.address,
    phone: data.phone,
    tableQueueEnabled: data.tableQueueEnabled,
    pickupQueueEnabled: data.pickupQueueEnabled,
    logoUrl: data.logoUrl,
    reviewNote: data.reviewNote,
  };
}

export function decodePlatformAdminSession(
  data: unknown,
): PlatformAdminSession {
  if (
    !isRecord(data) ||
    typeof data.name !== "string" ||
    data.name.trim() === "" ||
    typeof data.email !== "string" ||
    data.email.trim() === ""
  ) {
    throw new TypeError("The platform administrator response is invalid.");
  }

  return { name: data.name, email: data.email };
}

export function decodeOwnerAccountSession(data: unknown): OwnerAccountSession {
  if (
    !isRecord(data) ||
    typeof data.name !== "string" ||
    data.name.trim() === "" ||
    typeof data.email !== "string" ||
    data.email.trim() === ""
  ) {
    throw new TypeError("The owner account response is invalid.");
  }

  return { name: data.name, email: data.email };
}

export function decodeApprovedApplication(data: unknown): ApprovedApplication {
  if (
    !isRecord(data) ||
    typeof data.businessName !== "string" ||
    data.businessName.trim() === "" ||
    typeof data.slug !== "string" ||
    data.slug.trim() === ""
  ) {
    throw new TypeError("The approval response is invalid.");
  }

  return { businessName: data.businessName, slug: data.slug };
}

export function decodeEmptyObject(data: unknown): Record<string, never> {
  if (!isRecord(data) || Object.keys(data).length > 0) {
    throw new TypeError("The server response is invalid.");
  }

  return {};
}
