// @ts-check
import { defineConfig } from 'astro/config';
import alpinejs from '@astrojs/alpinejs';

// https://astro.build/config
export default defineConfig({
  site: 'https://cabbage.town',
  integrations: [alpinejs({ entrypoint: '/src/entrypoint' })],
  devToolbar: { enabled: false },
});
