import type { Metadata } from "next";
import { TicketStatusPage } from "./ticket-status";

type TicketPageProps = {
  params: Promise<{ token: string }>;
};

export const metadata: Metadata = {
  title: "Your restaurant ticket",
  description: "Check the status and progress of your restaurant ticket.",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default async function TicketPage({ params }: TicketPageProps) {
  const { token } = await params;

  return <TicketStatusPage token={token} />;
}
