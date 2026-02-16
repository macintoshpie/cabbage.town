import type { Loader } from 'astro/loaders';
import { readFile } from 'node:fs/promises';

interface RawRecording {
  key: string;
  post?: {
    id: string;
    title: string;
    slug: string;
    markdown: string;
    author: string;
    createdAt: string;
    updatedAt: string;
    tags: string[];
    category: string;
    excerpt: string;
  };
}

export function postsLoader(): Loader {
  return {
    name: 'posts-loader',
    async load({ store, renderMarkdown, parseData, generateDigest }) {
      const raw = await readFile('src/data/recordings.json', 'utf-8');
      const recordings: RawRecording[] = JSON.parse(raw);

      store.clear();

      for (const rec of recordings) {
        if (!rec.post) continue;
        const post = rec.post;

        const data = await parseData({
          id: post.id,
          data: {
            title: post.title,
            slug: post.slug,
            author: post.author,
            createdAt: post.createdAt,
            updatedAt: post.updatedAt,
            tags: post.tags,
            category: post.category,
            excerpt: post.excerpt,
            recordingKey: rec.key,
          },
        });

        // Convert single newlines to double so each line renders as its own paragraph
        // (source content uses \n between lines that authors intend as separate blocks)
        const md = post.markdown?.replace(/\n/g, '\n\n') ?? '';

        store.set({
          id: post.id,
          data,
          body: md,
          rendered: md ? await renderMarkdown(md) : undefined,
          digest: generateDigest({ ...post }),
        });
      }
    },
  };
}
