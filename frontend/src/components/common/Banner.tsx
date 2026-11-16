import type { ReactNode } from "react";
import { IconCross } from "./icons";

type BannerTone = "error" | "warning" | "info" | "success";

interface BannerProps {
  tone?: BannerTone;
  children: ReactNode;
  action?: ReactNode;
  onDismiss?: () => void;
}

export function Banner({ tone = "info", children, action, onDismiss }: BannerProps) {
  return (
    <div className={`banner banner--${tone}`} role={tone === "error" ? "alert" : "status"}>
      <div className="banner__content">{children}</div>
      {action ? <div className="banner__action">{action}</div> : null}
      {onDismiss ? (
        <button
          type="button"
          className="icon-button banner__dismiss"
          aria-label="Dismiss message"
          onClick={onDismiss}
        >
          <IconCross />
        </button>
      ) : null}
    </div>
  );
}
