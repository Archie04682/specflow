import { useState } from "react";
import { shortId } from "../../lib/format";
import type { ServerIdentity } from "../../types/server";
import { Banner } from "../common/Banner";

interface ServerBuildWarningProps {
  identity: ServerIdentity | null;
}

// Warns when the running API build is behind the checkout it was launched from.
// Dismissal lasts for the page and is keyed by the running process, so a
// restarted server (a new start time or revision) warns again.
export function ServerBuildWarning({ identity }: ServerBuildWarningProps) {
  const [dismissedKey, setDismissedKey] = useState<string | null>(null);

  const reason = identity?.checkout?.staleReason;
  if (!identity || !reason) return null;

  const key = `${identity.startedAt ?? ""}|${identity.revision ?? ""}`;
  if (dismissedKey === key) return null;

  const checkoutRevision = identity.checkout?.revision;
  const running = (
    <span className="mono">
      {identity.revision ? shortId(identity.revision) : "unknown build"}
    </span>
  );

  return (
    <div className="workspace-banner">
      <Banner tone="warning" onDismiss={() => setDismissedKey(key)}>
        {reason === "checkout-ahead" ? (
          <>
            The running API build {running} is behind this checkout
            {checkoutRevision ? (
              <> (<span className="mono">{shortId(checkoutRevision)}</span>)</>
            ) : null}
            . Restart the SpecFlow server to load the current code.
          </>
        ) : reason === "sources-newer" ? (
          <>
            The running API build {running} predates backend source changes.
            Restart the SpecFlow server to load the current code.
          </>
        ) : (
          <>
            The running API build {running} is stale. Restart the SpecFlow server
            to load the current code.
          </>
        )}
      </Banner>
    </div>
  );
}
