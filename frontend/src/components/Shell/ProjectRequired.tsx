import { useState, type FormEvent, type ReactNode } from "react";
import { Button } from "../common/Button";
import { ProjectPickerField } from "../common/ProjectPickerField";

interface ProjectRequiredProps {
  title: string;
  description: ReactNode;
  initialValue?: string;
  confirmLabel?: string;
  secondaryAction?: ReactNode;
  onApply: (project: string) => void;
}

// Deep links and first launches land here before any catalog can exist. The
// inline picker keeps the deep link intact while the operator supplies the
// repository once.
export function ProjectRequired({
  title,
  description,
  initialValue = "",
  confirmLabel = "Open project",
  secondaryAction,
  onApply,
}: ProjectRequiredProps) {
  const [value, setValue] = useState(initialValue);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = value.trim();
    if (!trimmed) return;
    onApply(trimmed);
  };

  return (
    <div className="project-gate">
      <div className="project-gate__card">
        <p className="eyebrow">Project</p>
        <h2 className="project-gate__title">{title}</h2>
        <p className="project-gate__copy">{description}</p>
        <form className="project-gate__form" onSubmit={submit}>
          <ProjectPickerField
            id="project-gate-path"
            value={value}
            onChange={setValue}
            autoFocus
          />
          <div className="project-gate__actions">
            <Button type="submit" variant="primary" disabled={!value.trim()}>
              {confirmLabel}
            </Button>
            {secondaryAction}
          </div>
        </form>
      </div>
    </div>
  );
}
