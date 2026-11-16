import type { ReactNode } from "react";
import { IconPanelLeft } from "../common/icons";
import { useShellChrome } from "./ShellChrome";

interface ViewHeaderProps {
  title: ReactNode;
  subtitle?: ReactNode;
  badge?: ReactNode;
  actions?: ReactNode;
}

// One header grammar for every workspace view. The panel toggle appears only
// while the panel is hidden, so a view never duplicates a restore control the
// shell is already showing.
export function ViewHeader({ title, subtitle, badge, actions }: ViewHeaderProps) {
  const { panelHidden, openPanel } = useShellChrome();

  return (
    <header className="view-header">
      <div className="view-header__leading">
        {panelHidden ? (
          <button
            type="button"
            className="icon-button view-header__panel-toggle"
            aria-label="Show control panel"
            onClick={openPanel}
          >
            <IconPanelLeft />
          </button>
        ) : null}
        <div className="view-header__titles">
          <div className="view-header__title-row">
            <h1 className="view-header__title">{title}</h1>
            {badge ? <div className="view-header__badge">{badge}</div> : null}
          </div>
          {subtitle ? <div className="view-header__subtitle">{subtitle}</div> : null}
        </div>
      </div>
      {actions ? <div className="view-header__actions">{actions}</div> : null}
    </header>
  );
}
