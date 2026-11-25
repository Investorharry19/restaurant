import type { Metadata } from "next";
import { RestaurantOnboarding } from "./restaurant-onboarding";

export const metadata: Metadata = {
  title: "Bring your restaurant online",
  description:
    "Sign in with a verified owner account to apply for restaurant queues.",
};

export default function OnboardingPage() {
  return <RestaurantOnboarding />;
}
