import { defineCollection, z } from 'astro:content';
import { postsLoader } from './loaders/posts-loader';

const posts = defineCollection({
  loader: postsLoader(),
  schema: z.object({
    title: z.string(),
    slug: z.string(),
    author: z.string(),
    createdAt: z.coerce.date(),
    updatedAt: z.coerce.date(),
    tags: z.array(z.string()),
    category: z.string(),
    excerpt: z.string(),
    recordingKey: z.string().optional(),
  }),
});

export const collections = { posts };
