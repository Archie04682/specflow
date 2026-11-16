import { formatDateTime, shortId } from "../../lib/format";
import type { ServerIdentity } from "../../types/server";

interface ServerSectionProps {
  identity: ServerIdentity | null;
  // loading is the first read in flight: "unavailable" would be a lie until it
  // finishes.
  loading?: boolean;
}

// The running server's checkout state in the operator's words.
function checkoutState(identity: ServerIdentity): string {
  const checkout = identity.checkout;
  if (!checkout) return "not running from source";
  if (checkout.staleReason === "checkout-ahead") {
    return checkout.revision
      ? `restart needed — behind checkout ${shortId(checkout.revision)}`
      : "restart needed — behind the checkout";
  }
  if (checkout.staleReason === "sources-newer") {
    return "restart needed — sources changed since the build";
  }
  return checkout.revision ? `up to date (${shortId(checkout.revision)})` : "up to date";
}

// Settings' view of which build is answering, and whether it is current.
export function ServerSection({ identity, loading = false }: ServerSectionProps) {
  return (
    <section className="settings-section" aria-label="Server">
      <h3 className="settings-section__title">Server</h3>
      {loading ? (
        <p className="settings-section__empty">Reading the server identity…</p>
      ) : identity ? (
        <dl className="settings-server">
          <div className="settings-server__row">
            <dt className="settings-server__label">Build</dt>
            <dd className="settings-server__value">
              {identity.revision ? (
                <span className="mono" title={identity.revision}>
                  {shortId(identity.revision)}
                </span>
              ) : (
                "unknown"
              )}
              {identity.modified ? (
                <span className="settings-server__flag">dirty tree</span>
              ) : null}
            </dd>
          </div>
          <div className="settings-server__row">
            <dt className="settings-server__label">Built</dt>
            <dd className="settings-server__value">
              {formatDateTime(identity.builtAt)}
            </dd>
          </div>
          <div className="settings-server__row">
            <dt className="settings-server__label">Started</dt>
            <dd className="settings-server__value">
              {formatDateTime(identity.startedAt)}
            </dd>
          </div>
          <div className="settings-server__row">
            <dt className="settings-server__label">Checkout</dt>
            <dd className="settings-server__value">{checkoutState(identity)}</dd>
          </div>
        </dl>
      ) : (
        <p className="settings-section__empty">
          The server identity is unavailable.
        </p>
      )}
    </section>
  );
}
