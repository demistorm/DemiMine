<script lang="ts">
	import { onDestroy } from 'svelte';
	import { api } from '$lib/api';

	export let targetId: number;
	export let targetType: 'server' | 'proxy';
	export let isRunning: boolean;

	let profiling = false;
	let startTime: Date | null = null;
	let elapsed = '0:00';
	let elapsedInterval: ReturnType<typeof setInterval> | null = null;

	async function startProfile() {
		try {
			const endpoint = targetType === 'server' ? `/api/servers/${targetId}/spark/profile/start` : `/api/proxies/${targetId}/spark/profile/start`;
			await api.post(endpoint);
			profiling = true;
			startTime = new Date();
			startElapsedTimer();
		} catch (err) {
			console.error('Failed to start profiling:', err);
		}
	}

	async function stopAndView() {
		try {
			const endpoint = targetType === 'server' ? `/api/servers/${targetId}/spark/profile/stop` : `/api/proxies/${targetId}/spark/profile/stop`;
			const response = await api.post(endpoint) as { url?: string };
			stopElapsedTimer();
			profiling = false;
			startTime = null;

			if (response?.url) {
				window.open(response.url, '_blank');
			}
		} catch (err) {
			console.error('Failed to stop profiling:', err);
		}
	}

	function startElapsedTimer() {
		if (elapsedInterval) clearInterval(elapsedInterval);
		elapsedInterval = setInterval(() => {
			if (startTime) {
				const now = new Date();
				const diff = Math.floor((now.getTime() - startTime.getTime()) / 1000);
				const minutes = Math.floor(diff / 60);
				const seconds = diff % 60;
				elapsed = `${minutes}:${seconds.toString().padStart(2, '0')}`;
			}
		}, 1000);
	}

	function stopElapsedTimer() {
		if (elapsedInterval) {
			clearInterval(elapsedInterval);
			elapsedInterval = null;
		}
	}

	onDestroy(() => {
		stopElapsedTimer();
	});
</script>

{#if isRunning}
	{#if profiling}
		<button class="profile-btn active" on:click={stopAndView}>
			Stop & View ({elapsed})
		</button>
	{:else}
		<button class="profile-btn" on:click={startProfile}>
			Generate Report
		</button>
	{/if}
{/if}

<style>
	.profile-btn {
		padding: 0.5rem 1rem;
		border-radius: 0.375rem;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		border: 1px solid var(--border);
		background-color: var(--bg-tertiary);
		color: var(--text-primary);
		transition: all 0.2s;
	}

	.profile-btn:hover:not(:disabled) {
		background-color: var(--border);
	}

	.profile-btn.active {
		background-color: var(--accent);
		color: white;
		border-color: var(--accent);
	}

	.profile-btn.active:hover:not(:disabled) {
		opacity: 0.9;
	}

	.profile-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
