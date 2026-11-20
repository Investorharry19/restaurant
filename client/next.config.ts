import type { NextConfig } from "next";

const apiInternalURL = new URL(
  process.env.API_INTERNAL_URL ?? "http://localhost:8080",
);

if (
  !["http:", "https:"].includes(apiInternalURL.protocol) ||
  apiInternalURL.username !== "" ||
  apiInternalURL.password !== "" ||
  apiInternalURL.pathname !== "/" ||
  apiInternalURL.search !== "" ||
  apiInternalURL.hash !== ""
) {
  throw new Error(
    "API_INTERNAL_URL must be an HTTP(S) origin without credentials or a path.",
  );
}

const nextConfig: NextConfig = {
  /* config options here */
  cacheComponents: true,
  partialPrefetching: true,
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${apiInternalURL.origin}/api/:path*`,
      },
    ];
  },
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
