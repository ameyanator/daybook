import type { DaybookEvent } from './api';

export type DaySection = { key: string; title: string; data: DaybookEvent[] };

const pad = (n: number) => String(n).padStart(2, '0');

/** Local calendar date as "YYYY-MM-DD". */
export const toDateString = (d: Date) =>
  `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** "YYYY-MM-DD" → local midnight of that date. */
export const fromDateString = (s: string) => {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
};

const dayOf = (e: DaybookEvent) => (e.kind === 'allDay' ? e.date : toDateString(e.startsAt));

function dayTitle(day: string, now: Date): string {
  const d = fromDateString(day);
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const diffDays = Math.round((d.getTime() - today.getTime()) / 86_400_000);
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

/**
 * Groups events by local calendar day, days in order. Within a day, all-day
 * items come first, then timed items chronologically.
 */
export function groupByDay(events: DaybookEvent[], now = new Date()): DaySection[] {
  const sorted = [...events].sort((a, b) => {
    const byDay = dayOf(a).localeCompare(dayOf(b));
    if (byDay) return byDay;
    if (a.kind !== b.kind) return a.kind === 'allDay' ? -1 : 1;
    if (a.kind === 'timed' && b.kind === 'timed') {
      const byTime = a.startsAt.getTime() - b.startsAt.getTime();
      if (byTime) return byTime;
    }
    return a.id - b.id;
  });
  const sections: DaySection[] = [];
  for (const e of sorted) {
    const key = dayOf(e);
    let section = sections[sections.length - 1];
    if (!section || section.key !== key) {
      section = { key, title: dayTitle(key, now), data: [] };
      sections.push(section);
    }
    section.data.push(e);
  }
  return sections;
}

/** Whether an item is over: timed items after their time, all-day after their day. */
export function isPast(e: DaybookEvent, now = new Date()): boolean {
  return e.kind === 'timed'
    ? e.startsAt.getTime() < now.getTime()
    : e.date < toDateString(now);
}

/** Next round hour from now — a sensible default for a new entry. */
export function nextHour(now = new Date()): Date {
  const d = new Date(now);
  d.setHours(d.getHours() + 1, 0, 0, 0);
  return d;
}
