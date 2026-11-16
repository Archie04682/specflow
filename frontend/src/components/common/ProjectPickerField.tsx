import { useState } from "react";
import { pickProject } from "../../api/picker";
import { describeError } from "../../lib/errors";
import { Button } from "./Button";

interface ProjectPickerFieldProps {
  id: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
}

// The manual path stays the primary contract: the native dialog writes into the
// same value, and every failure leaves the field editable.
export function ProjectPickerField({
  id,
  value,
  onChange,
  disabled = false,
  autoFocus = false,
}: ProjectPickerFieldProps) {
  const [picking, setPicking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const browse = async () => {
    setPicking(true);
    setError(null);
    try {
      const result = await pickProject();
      if (result.path) onChange(result.path);
    } catch (cause) {
      setError(describeError(cause));
    } finally {
      setPicking(false);
    }
  };

  return (
    <div className="field">
      <label className="field__label" htmlFor={id}>
        Project
      </label>
      <div className="field__row field__row--input">
        <input
          id={id}
          className="input mono"
          type="text"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder="/absolute/path/to/repository"
          spellCheck={false}
          autoComplete="off"
          autoFocus={autoFocus}
          required
        />
        <Button
          variant="secondary"
          onClick={() => void browse()}
          loading={picking}
          disabled={disabled}
        >
          Browse…
        </Button>
      </div>
      <p className="field__hint">
        A clean git repository. The path is stored in this browser and sent with
        every request.
      </p>
      {error ? (
        <p className="field__error" role="alert">
          Could not open the folder dialog: {error}. Type the path manually, or
          check that the server is running in your desktop session.
        </p>
      ) : null}
    </div>
  );
}
