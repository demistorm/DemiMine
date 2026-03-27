import { readable } from 'svelte/store';
import { browser } from '$app/environment';

export type InputMode = 'desktop' | 'mobile';

export const inputMode = readable<InputMode>('desktop', (set) => {
	if (!browser) return;

	const mq = window.matchMedia('(hover: none) and (pointer: coarse)');
	set(mq.matches ? 'mobile' : 'desktop');

	const handler = (e: MediaQueryListEvent) => set(e.matches ? 'mobile' : 'desktop');
	mq.addEventListener('change', handler);

	return () => mq.removeEventListener('change', handler);
});
