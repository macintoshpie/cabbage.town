import recordingsData from '../data/recordings.json';

export type DeduplicatedShow = {
  show: string;
  displayName: string;
  dj: string;
  date: string;
  keys: string[];
  postSlug?: string;
  postExcerpt?: string;
};

export function getDeduplicatedShows(): DeduplicatedShow[] {
  const recordings = [...recordingsData].sort(
    (a, b) => new Date(b.lastModified).getTime() - new Date(a.lastModified).getTime()
  );

  const deduped: DeduplicatedShow[] = [];
  const seen = new Set<string>();

  for (const rec of recordings) {
    const groupKey = `${rec.show}::${rec.date}`;
    if (seen.has(groupKey)) {
      const existing = deduped.find(d => `${d.show}::${d.date}` === groupKey);
      if (existing) existing.keys.push(rec.key);
      continue;
    }
    seen.add(groupKey);
    deduped.push({
      show: rec.show,
      displayName: rec.displayName,
      dj: rec.dj,
      date: rec.date,
      keys: [rec.key],
      postSlug: (rec as any).post?.slug,
      postExcerpt: (rec as any).post?.excerpt,
    });
  }

  return deduped;
}

/** DJs that shouldn't appear in the homepage "recent shows" list. Matched case-insensitively. */
const HOMEPAGE_EXCLUDED_DJS = ['the cabbage'];

/**
 * Picks shows for the homepage by round-robin over DJs: one show per DJ
 * (ordered by each DJ's most recent show), then a second per DJ, and so on
 * until `limit` is reached. The result is re-sorted newest-first so it still
 * reads as a "recent shows" list, just with more variety across DJs.
 */
export function getHomepageShows(limit = 10): DeduplicatedShow[] {
  const excluded = new Set(HOMEPAGE_EXCLUDED_DJS.map(d => d.toLowerCase()));
  const deduped = getDeduplicatedShows().filter(e => !excluded.has(e.dj.toLowerCase()));

  // Map preserves insertion order, so DJs are ordered by their most recent show.
  const byDj = new Map<string, DeduplicatedShow[]>();
  for (const entry of deduped) {
    const key = entry.dj.toLowerCase();
    if (!byDj.has(key)) byDj.set(key, []);
    byDj.get(key)!.push(entry);
  }

  const picked: DeduplicatedShow[] = [];
  const queues = [...byDj.values()];
  for (let round = 0; picked.length < limit; round++) {
    const roundPicks = queues.filter(q => q.length > round).map(q => q[round]);
    if (roundPicks.length === 0) break;
    picked.push(...roundPicks.slice(0, limit - picked.length));
  }

  return picked.sort((a, b) => deduped.indexOf(a) - deduped.indexOf(b));
}

export function getDjColorClass(show: string): string {
  const s = show.toLowerCase();
  if (s.includes('mulch')) return 'mulch-channel';
  if (s.includes('reginajingles')) return 'reginajingles-channel';
  if (s.includes('wild') || s.includes('chicago')) return 'ben';
  if (s.includes('terminus')) return 'terminus';
  if (s.includes('late nights') || s.includes('nights like')) return 'nlt';
  if (s.includes('home cooking')) return 'home-cooking';
  if (s.includes('posted')) return 'posted-up';
  return '';
}

export function shortDate(date: string): string {
  const d = new Date(date);
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
}

export function getRecordingsByKey() {
  return new Map(recordingsData.map(r => [r.key, r]));
}
