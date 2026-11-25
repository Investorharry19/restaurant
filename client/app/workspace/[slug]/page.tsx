import type { Metadata } from "next";
import { Suspense } from "react";
import { OwnerWorkspace } from "./workspace";
import styles from "./workspace.module.css";

type WorkspacePageProps = {
  params: Promise<{ slug: string }>;
};

export const metadata: Metadata = {
  title: "Restaurant workspace",
  description: "Manage restaurant queues, orders, staff, and guest entry.",
  robots: { index: false, follow: false },
};

export default function WorkspacePage({ params }: WorkspacePageProps) {
  return (
    <Suspense fallback={<WorkspaceLoading />}>
      <WorkspaceContent params={params} />
    </Suspense>
  );
}

async function WorkspaceContent({ params }: WorkspacePageProps) {
  const { slug } = await params;
  return <OwnerWorkspace slug={slug} />;
}

function WorkspaceLoading() {
  return (
    <main className={styles.page}>
      <div className={styles.shell}>
        <p className={styles.loadingState} role="status">
          Loading your restaurant workspace…
        </p>
      </div>
    </main>
  );
}
