import type { ReactNode } from "react";

interface CardProps {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  as?: "section" | "article" | "div";
}

export function Card({ title, actions, children, className, as = "section" }: CardProps) {
  const Tag = as;
  return (
    <Tag className={["panel", className ?? ""].filter(Boolean).join(" ")}>
      {title || actions ? (
        <header className="panel__header">
          {title ? <h2 className="panel__title">{title}</h2> : <span />}
          {actions ? <div className="panel__actions">{actions}</div> : null}
        </header>
      ) : null}
      <div className="panel__body">{children}</div>
    </Tag>
  );
}
