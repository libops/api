// Helper functions

export function capitalize(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

export function singularize(str: string): string {
  const singular: Record<string, string> = {
    organizations: "organization",
    projects: "project",
    sites: "site",
    secrets: "secret",
    firewall: "firewall",
    members: "member",
  };
  return singular[str] || str.replace(/s$/, "");
}

export function showNotification(type: "success" | "error", message: string) {
  const notification = document.createElement("div");
  notification.className = `fixed top-4 right-4 px-6 py-4 rounded-lg shadow-lg z-50 ${
    type === "success" ? "bg-green-600 text-white" : "bg-red-600 text-white"
  }`;
  notification.textContent = message;

  document.body.appendChild(notification);

  setTimeout(() => {
    notification.remove();
  }, 3000);
}

export function copyToClipboard(text: string) {
  navigator.clipboard
    .writeText(text)
    .then(() => {
      showNotification("success", "ID copied to clipboard");
    })
    .catch(() => {
      showNotification("error", "Failed to copy");
    });
}
