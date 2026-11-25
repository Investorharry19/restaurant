import type { Metadata } from "next";
import { PlatformApplications } from "../platform-admin";

export const metadata: Metadata = {
  title: "Restaurant applications",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default function PlatformApplicationsPage() {
  return <PlatformApplications />;
}
