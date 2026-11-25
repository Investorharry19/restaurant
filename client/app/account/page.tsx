import type { Metadata } from "next";
import { OwnerAccount } from "./owner-account";

export const metadata: Metadata = {
  title: "Owner account",
  description: "Create or sign in to a verified restaurant owner account.",
};

export default function OwnerAccountPage() {
  return <OwnerAccount />;
}
