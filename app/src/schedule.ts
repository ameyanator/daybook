import type { DaybookEvent } from './api';

export type DaySection = { key: string; title: string; data: DaybookEvent[] };

const dayKey = (d: Date) => `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;

function dayTitle(d: Date, now: Date): string {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const that = new Date(d.getFullYear(), d.getMonth(), d.getDate());
  const diffDays = Math.round((that.getTime() - today.getTime()) / 86_400_000);
  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Tomorrow';
  if (diffDays === -1) return 'Yesterday';
  return d.toLocaleDateString(undefined, {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    ...(d.getFullYear() !== now.getFullYear() && { year: 'numeric' }),
  });
}

/** Sorts events chronologically and groups them by local calendar day. */
export function groupByDay(events: DaybookEvent[], now = new Date()): DaySection[] {
  const sorted = [...events].sort(
    (a, b) => a.startsAt.getTime() - b.startsAt.getTime() || a.id - b.id,
  );
  const sections: DaySection[] = [];
  for (const e of sorted) {
    const key = dayKey(e.startsAt);
    let section = sections[sections.length - 1];
    if (!section || section.key !== key) {
      section = { key, title: dayTitle(e.startsAt, now), data: [] };
      sections.push(section);
    }
    section.data.push(e);
  }
  return sections;
}

/** Next round hour from now — a sensible default for a new entry. */
export function nextHour(now = new Date()): Date {
  const d = new Date(now);
  d.setHours(d.getHours() + 1, 0, 0, 0);
  return d;
}
