import { useEffect, useRef, useState, type FormEvent } from "react";
import { projectLabels } from "../../lib/projects";
import type { ServerIdentity } from "../../types/server";
import { Button } from "../common/Button";
import { IconCheck, IconFolder, IconTrash } from "../common/icons";
import { ProjectPickerField } from "../common/ProjectPickerField";
import { ServerSection } from "./ServerSection";

interface SettingsDialogProps {
  open: boolean;
  projects: string[];
  activeProject: string;
  identity?: ServerIdentity | null;
  identityLoading?: boolean;
  onAdd: (project: string) => void;
  onSelect: (project: string) => void;
  onRemove: (project: string) => void;
  onClose: () => void;
}

// Application settings live here: the project registry that every catalog and
// composer reads from, and the running server's build identity. Adding,
// switching, and forgetting a checkout all happen in one place.
export function SettingsDialog({
  open,
  projects,
  activeProject,
  identity,
  identityLoading,
  onAdd,
  onSelect,
  onRemove,
  onClose,
}: SettingsDialogProps) {
  const [value, setValue] = useState("");
  const dialogRef = useRef<HTMLFormElement>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);
  const onCloseRef = useRef(onClose);

  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  useEffect(() => {
    if (!open) return;
    setValue("");
    previouslyFocused.current = document.activeElement as HTMLElement | null;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== "Tab") return;
      const focusable = Array.from(
        dialogRef.current?.querySelectorAll<HTMLElement>(
          'button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])',
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
      previouslyFocused.current?.focus();
    };
  }, [open]);

  if (!open) return null;

  const labels = projectLabels(projects);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = value.trim();
    if (!trimmed) return;
    onAdd(trimmed);
  };

  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <form
        ref={dialogRef}
        className="dialog dialog--settings"
        role="dialog"
        aria-modal="true"
        aria-labelledby="settings-dialog-title"
        onSubmit={submit}
      >
        <h2 id="settings-dialog-title" className="dialog__title">
          Settings
        </h2>
        <p className="dialog__lead">
          SpecFlow stores everything locally. Add the repositories you work in,
          forget the ones you no longer need, and switch the active project.
        </p>

        <section className="settings-section" aria-label="Projects">
          <h3 className="settings-section__title">Projects</h3>
          {projects.length === 0 ? (
            <p className="settings-section__empty">
              No projects yet. Add the first one below.
            </p>
          ) : (
            <ul className="settings-projects">
              {labels.map((label) => {
                const active = label.path === activeProject;
                return (
                  <li
                    key={label.path}
                    className={`settings-project${active ? " settings-project--active" : ""}`}
                  >
                    <button
                      type="button"
                      className="settings-project__select"
                      aria-current={active || undefined}
                      aria-label={`Use project ${label.name}${label.suffix ? ` (${label.suffix})` : ""}`}
                      onClick={() => onSelect(label.path)}
                    >
                      <IconFolder className="settings-project__icon" />
                      <span className="settings-project__text">
                        <span className="settings-project__name">
                          {label.name}
                          {label.suffix ? (
                            <span className="settings-project__suffix">
                              {" "}· {label.suffix}
                            </span>
                          ) : null}
                        </span>
                        <span
                          className="settings-project__path mono"
                          title={label.path}
                        >
                          {label.path}
                        </span>
                      </span>
                      {active ? (
                        <IconCheck className="settings-project__check" />
                      ) : null}
                    </button>
                    <button
                      type="button"
                      className="icon-button icon-button--sm settings-project__remove"
                      aria-label={`Remove ${label.name}${label.suffix ? ` (${label.suffix})` : ""} from the workbench`}
                      title="Remove from the workbench (files are not deleted)"
                      onClick={() => onRemove(label.path)}
                    >
                      <IconTrash />
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </section>

        <div className="settings-add">
          <h3 className="settings-section__title">Add a project</h3>
          <ProjectPickerField
            id="settings-project-path"
            value={value}
            onChange={setValue}
            autoFocus
          />
        </div>

        <ServerSection identity={identity ?? null} loading={identityLoading} />

        <div className="dialog__actions">
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button type="submit" variant="primary" disabled={!value.trim()}>
            Add project
          </Button>
        </div>
      </form>
    </div>
  );
}
