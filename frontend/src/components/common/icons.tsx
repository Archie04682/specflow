import type { ReactNode } from "react";

// Inline SVG icons: no emoji, no icon dependency, consistent 16px geometry.
interface IconProps {
  className?: string;
}

function Icon({ children, className }: IconProps & { children: ReactNode }) {
  return (
    <svg
      className={className}
      viewBox="0 0 16 16"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

export function IconCheck({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M3.5 8.5l3 3 6-7" />
    </Icon>
  );
}

export function IconCross({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M4 4l8 8M12 4l-8 8" />
    </Icon>
  );
}

export function IconCircle({ className }: IconProps) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="8" r="5" />
    </Icon>
  );
}

export function IconPlay({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M5.5 3.8l6.5 4.2-6.5 4.2z" fill="currentColor" stroke="none" />
    </Icon>
  );
}

export function IconPause({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M5.5 3.5v9M10.5 3.5v9" />
    </Icon>
  );
}

export function IconDash({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M4 8h8" />
    </Icon>
  );
}

export function IconQuestion({ className }: IconProps) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="8" r="5.2" />
      <path d="M6.2 6.2a1.9 1.9 0 1 1 2.6 1.8c-.6.3-.9.8-.9 1.4v.2" />
      <circle cx="8" cy="11.6" r="0.55" fill="currentColor" stroke="none" />
    </Icon>
  );
}

export function IconCancelled({ className }: IconProps) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="8" r="5" />
      <path d="M5 11l6-6" />
    </Icon>
  );
}

export function IconPlus({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M8 3.5v9M3.5 8h9" />
    </Icon>
  );
}

export function IconHistory({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M3.2 8a4.8 4.8 0 1 0 1.4-3.4" />
      <path d="M3 3.5V6h2.5" />
      <path d="M8 5.5V8l1.8 1.2" />
    </Icon>
  );
}

export function BrandMark({ className }: IconProps) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      width="20"
      height="20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      aria-hidden="true"
      focusable="false"
    >
      <circle cx="6" cy="6" r="2.4" />
      <circle cx="18" cy="12" r="2.4" />
      <circle cx="6" cy="18" r="2.4" />
      <path d="M8.1 7.2l7.6 3.6M8.1 16.8l7.6-3.6" />
    </svg>
  );
}


export function IconFlows({ className }: IconProps) {
  return (
    <Icon className={className}>
      <rect x="2.2" y="2.6" width="4.4" height="3.6" rx="1" />
      <rect x="9.4" y="9.8" width="4.4" height="3.6" rx="1" />
      <rect x="9.4" y="2.6" width="4.4" height="3.6" rx="1" />
      <path d="M6.6 4.4h2.8M11.6 6.2v3.6" />
    </Icon>
  );
}

export function IconFolder({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M2.3 4.6a1.3 1.3 0 0 1 1.3-1.3h2.3l1.3 1.5h5.2a1.3 1.3 0 0 1 1.3 1.3v5.6a1.3 1.3 0 0 1-1.3 1.3H3.6a1.3 1.3 0 0 1-1.3-1.3Z" />
    </Icon>
  );
}

export function IconPanelLeft({ className }: IconProps) {
  return (
    <Icon className={className}>
      <rect x="2.3" y="3" width="11.4" height="10" rx="1.6" />
      <path d="M6.4 3v10" />
    </Icon>
  );
}


export function IconChevronDown({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M4.2 6.2 8 10l3.8-3.8" />
    </Icon>
  );
}

export function IconArrowUp({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M8 12.6V3.6M3.9 7.7 8 3.5l4.1 4.2" />
    </Icon>
  );
}


export function IconChevronRight({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M6.2 4.2 10 8l-3.8 3.8" />
    </Icon>
  );
}

export function IconSettings({ className }: IconProps) {
  return (
    <Icon className={className}>
      <circle cx="8" cy="8" r="2.2" />
      <path d="M8 2.4v1.5M8 12.1v1.5M2.4 8h1.5M12.1 8h1.5M4.05 4.05l1.06 1.06M10.89 10.89l1.06 1.06M11.95 4.05l-1.06 1.06M5.11 10.89l-1.06 1.06" />
    </Icon>
  );
}

export function IconTrash({ className }: IconProps) {
  return (
    <Icon className={className}>
      <path d="M3.4 4.6h9.2M6.2 4.6V3.4a.8.8 0 0 1 .8-.8h2a.8.8 0 0 1 .8.8v1.2M5 4.6l.5 7.5a1 1 0 0 0 1 .9h3a1 1 0 0 0 1-.9l.5-7.5" />
    </Icon>
  );
}
