import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { IconChevronDown, IconCross } from "../common/icons";
import { Button } from "../common/Button";

export interface MultiSelectOption {
  value: string;
  label: string;
  description?: string;
  group?: string;
}

interface MultiSelectFieldProps {
  id: string;
  label: string;
  options: MultiSelectOption[];
  value: string[];
  onChange: (values: string[]) => void;
  placeholder?: string;
  hint?: ReactNode;
  emptyText?: string;
  custom?: {
    placeholder: string;
    addLabel: string;
  };
}

// A select with multiple values: the closed trigger shows the selection as
// chips and the panel is a checkbox list. The panel stays open while several
// values are picked; it closes on the trigger, outside click, or Escape.
export function MultiSelectField({
  id,
  label,
  options,
  value,
  onChange,
  placeholder = "Select…",
  hint,
  emptyText,
  custom,
}: MultiSelectFieldProps) {
  const [open, setOpen] = useState(false);
  const [customValue, setCustomValue] = useState("");
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      setOpen(false);
      triggerRef.current?.focus();
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const selected = new Set(value);
  const knownValues = new Set(options.map((option) => option.value));
  const unknownValues = value.filter((entry) => !knownValues.has(entry));
  const allOptions: MultiSelectOption[] = [
    ...options,
    ...unknownValues.map((entry) => ({
      value: entry,
      label: entry,
      group: "Custom",
    })),
  ];
  const selectedOptions = value.map(
    (entry) =>
      allOptions.find((option) => option.value === entry) ?? {
        value: entry,
        label: entry,
      },
  );
  const groups = groupOptions(allOptions);

  const toggle = (entry: string) => {
    const next = new Set(selected);
    if (next.has(entry)) next.delete(entry);
    else next.add(entry);
    onChange([...next]);
  };

  const addCustom = (event: FormEvent) => {
    event.preventDefault();
    const entry = customValue.trim();
    if (!entry) return;
    onChange([...new Set([...value, entry])]);
    setCustomValue("");
  };

  return (
    <div className="field multi-select" ref={rootRef}>
      <label className="field__label" htmlFor={id} id={`${id}-label`}>
        {label}
      </label>
      <div className={`multi-select__control${open ? " multi-select__control--open" : ""}`}>
        <button
          ref={triggerRef}
          id={id}
          type="button"
          className="multi-select__trigger"
          aria-expanded={open}
          aria-controls={`${id}-panel`}
          aria-labelledby={`${id}-label`}
          onClick={() => setOpen((current) => !current)}
        >
          {selectedOptions.length > 0 ? (
            <span className="multi-select__chips">
              {selectedOptions.map((option) => (
                <span className="multi-select__chip" key={option.value}>
                  {option.label}
                </span>
              ))}
            </span>
          ) : (
            <span className="multi-select__placeholder">{placeholder}</span>
          )}
          <IconChevronDown
            className={`multi-select__chevron${open ? " multi-select__chevron--open" : ""}`}
          />
        </button>

        {open ? (
          <div
            id={`${id}-panel`}
            className="multi-select__panel"
            role="group"
            aria-label={`${label} options`}
          >
            {allOptions.length === 0 ? (
              <p className="multi-select__empty">
                {emptyText ?? "No options are available."}
              </p>
            ) : (
              groups.map((group) => (
                <div className="multi-select__group" key={group.label ?? "options"}>
                  {group.label ? (
                    <p className="multi-select__group-label">{group.label}</p>
                  ) : null}
                  {group.options.map((option) => (
                    <label className="multi-select__option" key={option.value}>
                      <input
                        type="checkbox"
                        checked={selected.has(option.value)}
                        onChange={() => toggle(option.value)}
                      />
                      <span className="multi-select__option-text">
                        <span className="multi-select__option-label">
                          {option.label}
                        </span>
                        {option.description ? (
                          <span className="multi-select__option-description">
                            {option.description}
                          </span>
                        ) : null}
                      </span>
                    </label>
                  ))}
                </div>
              ))
            )}

            {custom ? (
              <form className="multi-select__custom" onSubmit={addCustom}>
                <label
                  className="visually-hidden"
                  htmlFor={`${id}-custom`}
                >
                  {custom.addLabel}
                </label>
                <input
                  id={`${id}-custom`}
                  className="input mono"
                  placeholder={custom.placeholder}
                  value={customValue}
                  onChange={(event) => setCustomValue(event.target.value)}
                />
                <Button
                  type="submit"
                  variant="secondary"
                  size="sm"
                  disabled={!customValue.trim()}
                >
                  Add
                </Button>
              </form>
            ) : null}

            <div className="multi-select__footer">
              <span className="field__hint">
                {value.length === 0
                  ? "Nothing selected."
                  : `${value.length} selected.`}
              </span>
              {value.length > 0 ? (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onChange([])}
                  aria-label={`Clear ${label}`}
                >
                  <IconCross /> Clear
                </Button>
              ) : null}
              <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
                Done
              </Button>
            </div>
          </div>
        ) : null}
      </div>
      {hint ? <p className="field__hint">{hint}</p> : null}
    </div>
  );
}

function groupOptions(options: MultiSelectOption[]) {
  const groups = new Map<string | undefined, MultiSelectOption[]>();
  for (const option of options) {
    const bucket = groups.get(option.group);
    if (bucket) bucket.push(option);
    else groups.set(option.group, [option]);
  }
  return [...groups.entries()].map(([label, entries]) => ({
    label,
    options: entries,
  }));
}
