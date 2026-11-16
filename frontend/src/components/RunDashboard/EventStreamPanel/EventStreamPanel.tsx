import { isTerminalRun } from "../../../types/run";
import { useRunRecord, useStreamItems, useStreamStatus } from "../../../stores/runStore";
import { EmptyState } from "../../common/EmptyState";
import { CONNECTION_LABEL, connectionTone } from "../connection";
import { EventList } from "./EventList";

export function EventStreamPanel() {
  const items = useStreamItems();
  const status = useStreamStatus();
  const run = useRunRecord();
  const tone = connectionTone(status);

  return (
    <section className="panel stream-panel" aria-label="Event stream">
      <header className="panel__header">
        <h2 className="panel__title">Activity</h2>
        <div className="stream-panel__meta">
          <span className="muted small">{items.length} events</span>
          <span className={`connection connection--${tone}`} role="status">
            <span className="connection__dot" aria-hidden="true" />
            <span>{CONNECTION_LABEL[status]}</span>
          </span>
        </div>
      </header>
      <div className="panel__body stream-panel__body">
        {items.length === 0 ? (
          <EmptyState
            title={
              run && isTerminalRun(run.status)
                ? "No events recorded"
                : "Waiting for events"
            }
            description={
              run && isTerminalRun(run.status)
                ? "This run has no retained event journal."
                : "The stream reports here as the workflow executes."
            }
          />
        ) : (
          <EventList items={items} />
        )}
      </div>
    </section>
  );
}
