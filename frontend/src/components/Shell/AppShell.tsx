import { useEffect, useRef, type ReactNode } from "react";

interface AppShellProps {
  panel: ReactNode;
  children: ReactNode;
  collapsed: boolean;
  panelHidden: boolean;
  drawerOpen: boolean;
  onCloseDrawer: () => void;
}

// The shell paints two tracks: the control panel and the workspace. A hidden
// panel leaves the workspace full width; under the narrow breakpoint the same
// panel is an overlay drawer with a scrim, Escape, and a small focus trap.
export function AppShell({
  panel,
  children,
  collapsed,
  panelHidden,
  drawerOpen,
  onCloseDrawer,
}: AppShellProps) {
  const panelRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (!drawerOpen) return;

    const previouslyFocused = document.activeElement as HTMLElement | null;
    const initialControl =
      panelRef.current?.querySelector<HTMLElement>("[data-panel-initial-focus]") ??
      panelRef.current?.querySelector<HTMLElement>(
        'button:not([disabled]), [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
    initialControl?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseDrawer();
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = Array.from(
        panelRef.current?.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );
      if (focusable.length === 0) return;
      const first = focusable[0]!;
      const last = focusable[focusable.length - 1]!;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      previouslyFocused?.focus();
    };
  }, [drawerOpen, onCloseDrawer]);

  return (
    <div
      className="shell"
      data-panel={collapsed ? "closed" : "open"}
      data-drawer={drawerOpen ? "open" : "closed"}
    >
      <a className="skip-link" href="#workspace">
        Skip to content
      </a>

      <aside
        id="control-panel"
        ref={panelRef}
        className="shell__panel"
        aria-label="Sessions and flows"
        aria-hidden={panelHidden || undefined}
        inert={panelHidden || undefined}
      >
        {panel}
      </aside>

      {drawerOpen ? (
        <button
          type="button"
          className="shell__scrim"
          aria-label="Close control panel"
          onClick={onCloseDrawer}
        />
      ) : null}

      <div className="shell__main">
        <main id="workspace" className="shell__workspace" tabIndex={-1}>
          {children}
        </main>
      </div>
    </div>
  );
}
