import { useCallback, useLayoutEffect, useRef, useState } from "react";
import {
  EARLIER_PAGE_ITEMS,
  INITIAL_VISIBLE_ITEMS,
  visibleItems,
  type StreamItem,
} from "../../../projection/eventProjection";
import { EventItem } from "./EventItem";

interface EventListProps {
  items: StreamItem[];
}

export function EventList({ items }: EventListProps) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [visibleCount, setVisibleCount] = useState(INITIAL_VISIBLE_ITEMS);
  const [stickToBottom, setStickToBottom] = useState(true);
  // Thinking bodies are collapsed by default. The expansion set lives here, not
  // in the memoized row, so a delta burst cannot reset what the operator opened.
  const [expandedIds, setExpandedIds] = useState<Set<number>>(() => new Set());
  const toggleThinking = useCallback((id: number) => {
    setExpandedIds((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const shown = visibleItems(items, visibleCount);
  const hidden = items.length - shown.length;

  // Follow the newest event only while the operator is at the bottom. The
  // effect runs on every item update, including a growing text block.
  useLayoutEffect(() => {
    const element = scrollerRef.current;
    if (!element || !stickToBottom) return;
    element.scrollTop = element.scrollHeight;
  }, [items, stickToBottom, visibleCount]);

  const handleScroll = () => {
    const element = scrollerRef.current;
    if (!element) return;
    const distance =
      element.scrollHeight - element.scrollTop - element.clientHeight;
    setStickToBottom(distance < 32);
  };

  const jumpToLive = () => {
    const element = scrollerRef.current;
    setStickToBottom(true);
    if (element) element.scrollTop = element.scrollHeight;
  };

  const showEarlier = () => {
    const element = scrollerRef.current;
    const previousHeight = element?.scrollHeight ?? 0;
    setStickToBottom(false);
    setVisibleCount((count) => count + EARLIER_PAGE_ITEMS);
    window.requestAnimationFrame(() => {
      const current = scrollerRef.current;
      if (current) current.scrollTop += current.scrollHeight - previousHeight;
    });
  };

  return (
    <div
      className="event-list"
      ref={scrollerRef}
      onScroll={handleScroll}
      role="log"
      aria-live="off"
      aria-label="Run events"
    >
      {hidden > 0 ? (
        <button type="button" className="link-button event-list__earlier" onClick={showEarlier}>
          Show earlier ({hidden})
        </button>
      ) : null}

      <ol className="event-list__items">
        {shown.map((item) => (
          <li key={item.id} className="event-list__item">
            <EventItem
              item={item}
              expanded={expandedIds.has(item.id)}
              onToggleThinking={toggleThinking}
            />
          </li>
        ))}
      </ol>

      {!stickToBottom ? (
        <button type="button" className="jump-live" onClick={jumpToLive}>
          Jump to live
        </button>
      ) : null}
    </div>
  );
}
