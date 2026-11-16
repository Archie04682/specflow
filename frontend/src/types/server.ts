// The running server's identity, as reported by GET /api/server. Every field
// is optional: the build omits what it cannot determine, so a client must not
// treat a missing value as empty.
export interface ServerCheckout {
  revision?: string;
  staleReason?: string;
  newerSources?: number;
}

export interface ServerIdentity {
  service?: string;
  version?: string;
  revision?: string;
  revisionTime?: string;
  modified?: boolean;
  builtAt?: string;
  startedAt?: string;
  checkout?: ServerCheckout;
}
