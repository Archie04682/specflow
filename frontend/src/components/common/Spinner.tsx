interface SpinnerProps {
  size?: "sm" | "md" | "lg";
  label?: string;
}

export function Spinner({ size = "md", label }: SpinnerProps) {
  return (
    <span className={`spinner spinner--${size}`} role={label ? "status" : undefined}>
      <span className="spinner__circle" aria-hidden="true" />
      {label ? <span className="spinner__label">{label}</span> : null}
      {label ? <span className="visually-hidden">Loading</span> : null}
    </span>
  );
}
