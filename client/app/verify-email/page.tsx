import type { Metadata } from "next";
import { VerifyOwnerEmail } from "./verify-owner-email";

export const metadata: Metadata = {
  title: "Verify your email",
  description: "Verify the email address for your restaurant owner account.",
  referrer: "no-referrer",
};

export default async function VerifyEmailPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string | string[] }>;
}) {
  const params = await searchParams;
  const token = typeof params.token === "string" ? params.token : "";
  return <VerifyOwnerEmail token={token} />;
}
