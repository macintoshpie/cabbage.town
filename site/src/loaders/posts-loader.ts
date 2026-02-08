import type { Loader } from 'astro/loaders';
import { readFile } from 'node:fs/promises';

interface RawPost {
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
  recordingKey?: string;
}

export function postsLoader(): Loader {
  return {
    name: 'posts-loader',
    async load({ store, renderMarkdown, parseData, generateDigest }) {
      const raw = await readFile('src/data/posts.json', 'utf-8');
      const posts: RawPost[] = JSON.parse(raw);

      store.clear();

      for (const post of posts) {
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
            recordingKey: post.recordingKey,
          },
        });

        store.set({
          id: post.id,
          data,
          body: post.markdown,
          rendered: post.markdown ? await renderMarkdown(post.markdown) : undefined,
          digest: generateDigest({ ...post }),
        });
      }
    },
  };
}
