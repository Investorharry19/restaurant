import type { Metadata } from "next";
import { Suspense } from "react";
import { AcceptStaffInvite } from "./accept-staff-invite";
import styles from "../../account/owner-account.module.css";

type AcceptInvitePageProps = {
  searchParams: Promise<{ token?: string | string[] }>;
};

export const metadata: Metadata = {
  title: "Accept staff invitation",
  description: "Join a restaurant workspace.",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default function AcceptInvitePage({
  searchParams,
}: AcceptInvitePageProps) {
  return (
    <Suspense fallback={<InviteLoading />}>
      <AcceptInviteContent searchParams={searchParams} />
    </Suspense>
  );
}

async function AcceptInviteContent({ searchParams }: AcceptInvitePageProps) {
  const params = await searchParams;
  const token = typeof params.token === "string" ? params.token : "";
  return <AcceptStaffInvite token={token} />;
}

function InviteLoading() {
  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <section className={styles.accountPanel} aria-labelledby="invite-title">
          <h1 id="invite-title">Join the restaurant team.</h1>
          <p className={styles.status} role="status">
            Checking your invitation…
          </p>
        </section>
      </div>
    </main>
  );
}
