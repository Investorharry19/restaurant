import type { Metadata } from "next";
import { PlatformAdminLogin } from "../platform-admin";

export const metadata: Metadata = {
  title: "Platform administrator sign in",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default function PlatformAdminLoginPage() {
  return <PlatformAdminLogin />;
}
