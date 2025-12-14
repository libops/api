// Main entry point for the dashboard application

import { initializeModal } from "@/utils/modal";
import { openCreateModal, openEditModal } from "@/forms/builder";
import { deleteResource } from "@/resources/operations";
import { copyToClipboard } from "@/utils/helpers";
import * as apiKeys from "@/api/apikeys";
import * as sshKeys from "@/api/sshkeys";

// Initialize the application
function init() {
  console.log("Dashboard application initialized");
  initializeModal();

  // Make functions available globally for inline onclick handlers
  (window as any).openCreateModal = openCreateModal;
  (window as any).openEditModal = openEditModal;
  (window as any).deleteResource = deleteResource;
  (window as any).copyToClipboard = copyToClipboard;

  // API management functions
  (window as any).apiKeys = apiKeys;
  (window as any).sshKeys = sshKeys;
}

// Run initialization when DOM is ready
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
