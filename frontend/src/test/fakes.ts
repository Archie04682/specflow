import { act } from "@testing-library/react";
import type { RunEvent } from "../types/events";

// A synchronous EventSource stand-in: tests decide exactly when the
// connection opens, an event arrives, or the transport fails.
export class FakeEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  static instances: FakeEventSource[] = [];

  static reset(): void {
    FakeEventSource.instances = [];
  }

  readonly url: string;
  readyState: number = FakeEventSource.CONNECTING;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  closed = false;

  private listeners = new Map<string, Set<EventListener>>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: EventListener): void {
    const set = this.listeners.get(type) ?? new Set<EventListener>();
    set.add(listener);
    this.listeners.set(type, set);
  }

  removeEventListener(type: string, listener: EventListener): void {
    this.listeners.get(type)?.delete(listener);
  }

  close(): void {
    this.closed = true;
    this.readyState = FakeEventSource.CLOSED;
  }

  open(): void {
    this.readyState = FakeEventSource.OPEN;
    this.onopen?.();
  }

  fail(readyState: number = FakeEventSource.CONNECTING): void {
    this.readyState = readyState;
    this.onerror?.();
  }

  emit(type: string, event: RunEvent): void {
    const message = { data: JSON.stringify(event) } as MessageEvent;
    for (const listener of this.listeners.get(type) ?? []) {
      listener(message);
    }
  }

  emitRaw(type: string, data: string): void {
    const message = { data } as MessageEvent;
    for (const listener of this.listeners.get(type) ?? []) {
      listener(message);
    }
  }
}

export function fakeFactory() {
  return (url: string) => new FakeEventSource(url) as unknown as EventSource;
}

export function lastSource(): FakeEventSource {
  const source = FakeEventSource.instances[FakeEventSource.instances.length - 1];
  if (!source) throw new Error("no EventSource was created");
  return source;
}

// The event hook batches a burst into the projection once per frame. Tests
// dispatch synchronously, then drain that frame inside act so React sees the
// resulting store update.
export async function settleStream(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => {
      requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
    });
  });
}
