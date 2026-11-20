# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Next.js App Router with strict TypeScript for customer and staff experiences; a Go modular monolith using only `net/http`; PostgreSQL with hand-written SQL, `pgx`, and database migrations.

## Users

- Customers joining a restaurant's table waitlist or tracking an order for pickup without creating a full account.
- Restaurant owners, managers, and staff operating queues, tables, and orders for their restaurant.
- Platform administrators managing restaurant availability and platform health.

## Product Purpose

Provide a multi-restaurant QR-based queue, table, order-status, and notification platform. Customers use a restaurant QR URL to join or track a queue and see durable ticket progress. Staff manage the queue and fulfill table and pickup workflows. Success means customers can wait away from the entrance or pickup counter while staff can safely operate queues across multiple restaurants.

## Positioning

One service supports both table waits and staff-entered pickup orders through a restaurant's QR entry point, with secure ticket access, persistent status, and optional real-time and push updates rather than requiring a full customer account.

## Operating Context

Customers primarily use a mobile browser after scanning a restaurant QR code. Restaurant staff use an authenticated dashboard on desktop or tablet. The platform is multi-tenant; restaurant-owned data and staff actions must remain scoped to the authorized restaurant.

## Capabilities and Constraints

- The initial release covers restaurant profiles, staff access, table and pickup queues, tickets, table and order operations, real-time updates, and optional Web Push notifications.
- The Go API owns business rules and persistence. The Next.js app does not connect to PostgreSQL or contain secrets.
- PostgreSQL is the source of truth. Queue allocation, ticket transitions, table assignment, and order changes require transactional, concurrency-safe operations.
- Out of scope for the initial release: POS replacement, customer payment checkout, delivery management, native apps, loyalty or marketing campaigns, AI wait predictions, and a reservation marketplace.
- A named accessibility conformance level and the final product brand/name are not specified.

## Evidence on Hand

- `prd.md` is the product and engineering baseline.
- `design.md` is the required visual reference.
- No approved product logo, photography, customer testimonials, or other brand assets are provided. Do not invent endorsements or product claims.

## Product Principles

1. A customer can join and track a queue without creating a full account.
2. The backend is authoritative for state, identity, authorization, and restaurant tenancy.
3. Persistent ticket state remains available after disconnects; push and real-time updates improve awareness but are not required to use the service.
4. Queue and order transitions are explicit, auditable, and safe under concurrent staff actions.
5. Customer data is limited to what the service needs and is not exposed across customers or restaurants.

## Accessibility & Inclusion

Build responsive, semantic interfaces with accessible labels and usable loading, empty, success, and error states. The specification does not name a particular conformance level.
