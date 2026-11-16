import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent,
  type ReactNode,
} from "react";
import {
  DEFAULT_RUN_SIDE,
  MAX_RUN_SIDE,
  MIN_RUN_SIDE,
  clampRunSide,
} from "../../lib/split";
import { readStoredRunSplit, writeStoredRunSplit } from "../../lib/storage";

interface RunSplitProps {
  graph: ReactNode;
  side: ReactNode;
  busy?: boolean;
}

const KEYBOARD_STEP = 24;

/**
 * The run view's two panes with one adjustable separator. Width is a clamped
 * pixel value remembered per browser; the stacked layout hides the handle and
 * owns its own grid, so the choice cannot fight the responsive layout.
 */
export function RunSplit({ graph, side, busy }: RunSplitProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(() => readStoredRunSplit() ?? DEFAULT_RUN_SIDE);
  const [containerWidth, setContainerWidth] = useState(0);
  const [dragging, setDragging] = useState(false);
  const dragStart = useRef<{ x: number; width: number } | null>(null);
  const widthRef = useRef(width);
  widthRef.current = width;

  const applyWidth = useCallback(
    (next: number, persist: boolean) => {
      const clamped = clampRunSide(next, containerWidth);
      setWidth(clamped);
      if (persist) writeStoredRunSplit(clamped);
    },
    [containerWidth],
  );

  // Track the container so a shrinking window re-clamps the chosen width
  // instead of squeezing the graph below its minimum.
  useEffect(() => {
    const element = containerRef.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => {
      const measured = Math.round(element.getBoundingClientRect().width);
      setContainerWidth(measured);
      setWidth((current) => {
        const clamped = clampRunSide(current, measured);
        return current === clamped ? current : clamped;
      });
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  // Seed the container width from the first layout so a remembered width is
  // clamped before the first paint, not only after the observer fires.
  useLayoutEffect(() => {
    const element = containerRef.current;
    if (!element) return;
    const measured = Math.round(element.getBoundingClientRect().width);
    if (measured <= 0) return;
    setContainerWidth(measured);
    setWidth((current) => clampRunSide(current, measured));
  }, []);

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    dragStart.current = { x: event.clientX, width: widthRef.current };
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setDragging(true);
  };

  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const start = dragStart.current;
    if (!start) return;
    applyWidth(start.width - (event.clientX - start.x), false);
  };

  const endDrag = (event: PointerEvent<HTMLDivElement>) => {
    if (!dragStart.current) return;
    dragStart.current = null;
    event.currentTarget.releasePointerCapture?.(event.pointerId);
    setDragging(false);
    writeStoredRunSplit(widthRef.current);
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    switch (event.key) {
      case "ArrowLeft":
        applyWidth(widthRef.current + KEYBOARD_STEP, true);
        break;
      case "ArrowRight":
        applyWidth(widthRef.current - KEYBOARD_STEP, true);
        break;
      case "Home":
        applyWidth(MIN_RUN_SIDE, true);
        break;
      case "End":
        applyWidth(MAX_RUN_SIDE, true);
        break;
      case "Enter":
        applyWidth(DEFAULT_RUN_SIDE, true);
        break;
      default:
        return;
    }
    event.preventDefault();
  };

  const maxWidth = clampRunSide(MAX_RUN_SIDE, containerWidth);

  return (
    <main
      className={dragging ? "run-body run-body--resizing" : "run-body"}
      ref={containerRef}
      aria-busy={busy || undefined}
      style={{ "--run-side": `${width}px` } as CSSProperties}
    >
      {graph}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize activity panel"
        aria-valuemin={MIN_RUN_SIDE}
        aria-valuemax={maxWidth}
        aria-valuenow={width}
        title="Drag to resize; double-click or press Enter to reset"
        tabIndex={0}
        className={dragging ? "run-split__handle run-split__handle--dragging" : "run-split__handle"}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onKeyDown={onKeyDown}
        onDoubleClick={() => applyWidth(DEFAULT_RUN_SIDE, true)}
      />
      {side}
    </main>
  );
}
