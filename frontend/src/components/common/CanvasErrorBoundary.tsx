import { Component, type ErrorInfo, type ReactNode } from "react";
import { Button } from "./Button";

interface CanvasErrorBoundaryProps {
  children: ReactNode;
  resetKey: string;
}

interface CanvasErrorBoundaryState {
  failed: boolean;
}

/**
 * A canvas fault must not blank the whole editor: the header, tabs, and
 * inspector stay usable, and the user can retry or reopen the workflow.
 */
export class CanvasErrorBoundary extends Component<
  CanvasErrorBoundaryProps,
  CanvasErrorBoundaryState
> {
  state: CanvasErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): CanvasErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Flow canvas failed to render", error, info.componentStack);
  }

  componentDidUpdate(previous: CanvasErrorBoundaryProps) {
    if (this.state.failed && previous.resetKey !== this.props.resetKey) {
      this.setState({ failed: false });
    }
  }

  render() {
    if (!this.state.failed) return this.props.children;

    return (
      <div className="empty-state" role="alert">
        <p className="empty-state__title">The graph could not be rendered</p>
        <p className="empty-state__description">
          Your draft is still here. Try the canvas again, or reopen the
          workflow from the start screen.
        </p>
        <div className="empty-state__action">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => this.setState({ failed: false })}
          >
            Try again
          </Button>
        </div>
      </div>
    );
  }
}
