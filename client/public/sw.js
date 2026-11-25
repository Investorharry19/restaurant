self.addEventListener("push", (event) => {
  let payload = {};
  if (event.data) {
    try {
      payload = event.data.json();
    } catch {
      payload = { body: event.data.text() };
    }
  }

  const title =
    typeof payload.title === "string" ? payload.title : "Ticket update";
  const body =
    typeof payload.body === "string"
      ? payload.body
      : "There is an update to your restaurant ticket.";
  const tag = typeof payload.tag === "string" ? payload.tag : "ticket-update";
  const url = typeof payload.url === "string" ? payload.url : "/";

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      data: { url },
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const target = new URL(
    typeof event.notification.data?.url === "string"
      ? event.notification.data.url
      : "/",
    self.location.origin,
  );
  const safeTarget =
    target.origin === self.location.origin ? target.href : self.location.origin;

  event.waitUntil(
    self.clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then((clients) => {
        for (const client of clients) {
          if ("focus" in client) {
            return client.navigate(safeTarget).then(() => client.focus());
          }
        }
        return self.clients.openWindow(safeTarget);
      }),
  );
});

self.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    (async () => {
      try {
        let subscription = event.newSubscription;
        if (!subscription) {
          const response = await fetch("/api/public/push/vapid-key", {
            cache: "no-store",
            credentials: "same-origin",
            headers: { Accept: "application/json" },
          });
          if (!response.ok) {
            throw new Error("Could not load the push subscription key.");
          }
          const result = await response.json();
          const publicKey = result?.data?.publicKey;
          if (result?.success !== true || typeof publicKey !== "string") {
            throw new Error("The push subscription key response is invalid.");
          }
          const base64 = publicKey.replace(/-/g, "+").replace(/_/g, "/");
          const applicationServerKey = Uint8Array.from(
            atob(base64),
            (character) => character.charCodeAt(0),
          );
          subscription = await self.registration.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey,
          });
        }

        const clients = await self.clients.matchAll({
          type: "window",
          includeUncontrolled: true,
        });
        const previousEndpoint = event.oldSubscription?.endpoint;
        for (const client of clients) {
          client.postMessage({
            type: "PUSH_SUBSCRIPTION_CHANGED",
            previousEndpoint,
          });
        }
      } catch (error) {
        const clients = await self.clients.matchAll({
          type: "window",
          includeUncontrolled: true,
        });
        for (const client of clients) {
          client.postMessage({ type: "PUSH_SUBSCRIPTION_REFRESH_FAILED" });
        }
        throw error;
      }
    })(),
  );
});
