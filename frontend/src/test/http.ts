import { vi } from "vitest";

export function fakeResponse(status: number, body?: unknown): Response {
  const text = body === undefined ? "" : JSON.stringify(body);
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => text,
  } as unknown as Response;
}

export function rawResponse(status: number, text: string): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => text,
  } as unknown as Response;
}

type FetchHandler = (
  url: string,
  init: RequestInit | undefined,
) => Response | Promise<Response>;

type FetchMock = {
  mock: { calls: unknown[][] };
};

export function stubFetch(handler: FetchHandler): FetchMock & ReturnType<typeof vi.fn> {
  const mock = vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
    Promise.resolve(handler(String(input), init)),
  );
  vi.stubGlobal("fetch", mock);
  return mock as unknown as FetchMock & ReturnType<typeof vi.fn>;
}

export function callUrl(mock: FetchMock, index = 0): string {
  return String(mock.mock.calls[index]?.[0] ?? "");
}

export function callBody(mock: FetchMock, index = 0): unknown {
  const init = mock.mock.calls[index]?.[1] as RequestInit | undefined;
  if (!init?.body) return undefined;
  return JSON.parse(String(init.body));
}

export function callMethod(mock: FetchMock, index = 0): string {
  const init = mock.mock.calls[index]?.[1] as RequestInit | undefined;
  return init?.method ?? "GET";
}

export function callCount(mock: FetchMock, matches: (url: string) => boolean): number {
  return mock.mock.calls.filter((call) => matches(String(call[0]))).length;
}
