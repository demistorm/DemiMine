/** @type {import('tailwindcss').Config} */
export default {
	content: ['./src/**/*.{html,js,svelte,ts}', './src/*.{html,js,svelte,ts}'],
	theme: {
		extend: {
			colors: {
				'bg-primary': '#0f0f0f',
				'bg-secondary': '#1a1a1a',
				'bg-tertiary': '#252525',
				'text-primary': '#ffffff',
				'text-secondary': '#a0a0a0',
				'accent': '#3b82f6',
				'accent-hover': '#2563eb',
				success: '#22c55e',
				warning: '#eab308',
				error: '#ef4444',
				border: '#333333'
			}
		}
	},
	plugins: []
};
