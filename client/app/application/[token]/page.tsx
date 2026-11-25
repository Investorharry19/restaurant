import type { Metadata } from "next";
import { ApplicationStatusPage } from "./application-status";

type ApplicationPageProps = {
  params: Promise<{ token: string }>;
};

export const metadata: Metadata = {
  title: "Application status",
  description: "Check the status of your restaurant application.",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default async function ApplicationPage({
  params,
}: ApplicationPageProps) {
  const { token } = await params;
  return <ApplicationStatusPage token={token} />;
}
