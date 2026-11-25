import type { Metadata } from "next";
import { Suspense } from "react";
import { RestaurantOnboarding } from "./restaurant-onboarding";
import styles from "./onboarding.module.css";

export const metadata: Metadata = {
  title: "Bring your restaurant online",
  description:
    "Sign in with a verified owner account to apply for restaurant queues.",
};

async function OnboardingContent({
  searchParams,
}: {
  searchParams: Promise<{ resubmit?: string | string[] }>;
}) {
  const { resubmit } = await searchParams;
  const resubmitToken = typeof resubmit === "string" ? resubmit : undefined;

  return <RestaurantOnboarding resubmitToken={resubmitToken} />;
}

export default function OnboardingPage({
  searchParams,
}: {
  searchParams: Promise<{ resubmit?: string | string[] }>;
}) {
  return (
    <Suspense
      fallback={
        <main className={styles.page}>
          <div className={styles.statusShell}>
            <div className={styles.statusHeader}>
              <h1>Owner application</h1>
              <p>Preparing your application…</p>
            </div>
          </div>
        </main>
      }
    >
      <OnboardingContent searchParams={searchParams} />
    </Suspense>
  );
}
