require "json"

module Web
  module PwaRoutes
    # Gripi mark: the pi CLI glyph (verbatim) held by four corner clamps.
    # Geometry lives on an 8x8 pixel grid, cell = 100. See branding/.
    GRIPI_ICON_GRIP = '<path fill="#F24405" d="M0 0H200V100H100V200H0ZM600 0H800V200H700V100H600ZM0 600H100V700H200V800H0ZM700 600H800V800H600V700H700Z"/>'
    GRIPI_ICON_PI = '<g fill="#F1EFE9" transform="translate(200 200)"><path fill-rule="evenodd" d="M0 0H300V200H200V300H100V400H0ZM100 100V200H200V100Z"/><path d="M300 200H400V400H300Z"/></g>'

    def self.registered(app)
      app.get "/manifest.webmanifest" do
        content_type "application/manifest+json"
        JSON.generate(
          name: "Gripi",
          short_name: "Gripi",
          start_url: "/",
          scope: "/",
          display: "standalone",
          background_color: "#18181e",
          theme_color: "#18181e",
          icons: [
            { src: "/app-icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
            { src: "/app-icon-maskable.svg", sizes: "any", type: "image/svg+xml", purpose: "maskable" }
          ]
        )
      end

      app.get "/app-icon.svg" do
        content_type "image/svg+xml"
        <<~SVG
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 800">
            <rect width="800" height="800" rx="96" fill="#18181e"/>
            <g transform="translate(80 80) scale(0.8)">#{GRIPI_ICON_GRIP}#{GRIPI_ICON_PI}</g>
          </svg>
        SVG
      end

      # Maskable variant keeps the mark inside the ~80% safe zone so round
      # masks never crop the corner clamps.
      app.get "/app-icon-maskable.svg" do
        content_type "image/svg+xml"
        <<~SVG
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 800">
            <rect width="800" height="800" fill="#18181e"/>
            <g transform="translate(176 176) scale(0.56)">#{GRIPI_ICON_GRIP}#{GRIPI_ICON_PI}</g>
          </svg>
        SVG
      end

      app.get "/service-worker.js" do
        content_type "application/javascript"
        headers "Cache-Control" => "no-cache"
        <<~JS
          self.addEventListener("install", (event) => {
            self.skipWaiting();
          });

          self.addEventListener("activate", (event) => {
            event.waitUntil(self.clients.claim());
          });

          self.addEventListener("message", (event) => {
            const data = event.data || {};
            if (!["gripi-notification", "gripi-notification-test"].includes(data.type)) return;

            const defaultUrl = data.type === "gripi-notification-test" ? "/notification-test" : "/";
            const defaultTag = data.type === "gripi-notification-test" ? "gripi-notification-test" : "gripi-notification";
            event.waitUntil(self.registration.showNotification(data.title || "Gripi", {
              body: data.body || "Notifications are working.",
              tag: data.tag || defaultTag,
              renotify: true,
              icon: "/app-icon.svg",
              badge: "/app-icon.svg",
              data: { url: data.url || defaultUrl }
            }));
          });

          self.addEventListener("notificationclick", (event) => {
            event.notification.close();
            const url = event.notification.data?.url || "/";
            event.waitUntil((async () => {
              const clientList = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
              for (const client of clientList) {
                if ("focus" in client) {
                  await client.focus();
                  if ("navigate" in client) await client.navigate(url);
                  return;
                }
              }
              if (self.clients.openWindow) await self.clients.openWindow(url);
            })());
          });
        JS
      end

      app.get "/notification-test" do
        erb :notification_test
      end
    end
  end
end
