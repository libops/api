// Modal utilities

export function closeModal() {
  const modal = document.getElementById("modal");
  if (modal) {
    modal.classList.add("hidden");
  }
}

export function openModal(title: string, content: HTMLElement | string) {
  const modal = document.getElementById("modal");
  const titleElement = document.getElementById("modal-title");
  const contentElement = document.getElementById("modal-content");

  if (!modal || !titleElement || !contentElement) {
    console.error("Modal elements not found");
    return;
  }

  titleElement.textContent = title;

  if (typeof content === "string") {
    contentElement.innerHTML = content;
  } else {
    contentElement.innerHTML = "";
    contentElement.appendChild(content);
  }

  modal.classList.remove("hidden");
}

export function showLoadingModal(title: string) {
  openModal(
    title,
    '<div class="text-center py-4"><div class="animate-spin rounded-full h-8 w-8 border-b-2 border-green-600 mx-auto"></div></div>'
  );
}

// Set up modal event listeners
export function initializeModal() {
  // Close modal on escape key
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      closeModal();
    }
  });

  // Close modal when clicking outside
  document.getElementById("modal")?.addEventListener("click", (e) => {
    if ((e.target as HTMLElement).id === "modal") {
      closeModal();
    }
  });
}
