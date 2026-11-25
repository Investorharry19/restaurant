import type { Metadata } from "next";
import { RestaurantLanding } from "./restaurant-landing";

type RestaurantPageProps = {
  params: Promise<{ slug: string }>;
};

export const metadata: Metadata = {
  title: "Restaurant queue",
  description: "Choose a restaurant queue service.",
};

export default async function RestaurantPage({ params }: RestaurantPageProps) {
  const { slug } = await params;

  return <RestaurantLanding slug={slug} />;
}
