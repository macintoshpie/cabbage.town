import recordingsData from '../data/recordings.json';

export type DeduplicatedShow = {
  show: string;
  displayName: string;
  dj: string;
  date: string;
  keys: string[];
  postSlug?: string;
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
    });
  }

  return deduped;
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
