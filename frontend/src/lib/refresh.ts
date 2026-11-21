/**
 * Project-scoped reads revalidate on window focus. A provisioning action
 * changes the project on disk, so it asks for that same revalidation rather
 * than making every hook expose a cross-surface refresh.
 */
export function refreshProjectData(): void {
  window.dispatchEvent(new Event("focus"));
}
