<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import '../app.css';
	import { page } from '$app/stores';
	import { token } from '$lib/stores/auth';
	import { resourcesApi, type SystemResources } from '$lib/api';

	let usedMB = 0;
	let maxMB = 0;
	let pollingInterval: number;

	async function fetchResources() {
		try {
			const resources = await resourcesApi.get();
			usedMB = resources.used_mb;
			maxMB = resources.max_mb;
		} catch (e) {
			console.error('Failed to fetch resources:', e);
		}
	}

	onMount(() => {
		fetchResources();
		pollingInterval = setInterval(fetchResources, 2000) as unknown as number;
	});

	onDestroy(() => {
		if (pollingInterval) {
			clearInterval(pollingInterval);
		}
	});

	function handleLogout() {
		token.set(null);
	}

	$: percentage = maxMB > 0 ? (usedMB / maxMB) * 100 : 0;
	$: barColor = percentage > 95 ? 'var(--error)' : percentage > 80 ? 'var(--warning)' : 'var(--accent)';
</script>

{#if $page.url.pathname !== '/login'}
	<nav class="top-nav">
		<div class="nav-brand">
			<a href="/" class="nav-link">
				<strong>DemiMine</strong>
			</a>
		</div>

		<div class="ram-indicator">
			<div class="ram-bar-container">
				<div class="ram-bar-fill" style="width: {percentage}%; background-color: {barColor};"></div>
			</div>
			<span class="ram-text">{usedMB} MB / {maxMB} MB</span>
		</div>

		<div class="nav-links">
			<a href="/settings" class="nav-link" class:active={$page.url.pathname === '/settings'}>
				Settings
			</a>
			<a href="/" class="nav-link" class:active={$page.url.pathname === '/'}>
				Servers
			</a>
			{#if $token}
				<button on:click={handleLogout} class="nav-link">
					Logout
				</button>
			{:else}
				<a href="/login" class="nav-link">
					Login
				</a>
			{/if}
		</div>
	</nav>
{/if}

<slot />

<style>
	.top-nav {
		background-color: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1.5rem;
		height: 56px;
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 100;
	}

	.nav-brand {
		display: flex;
		align-items: center;
	}

	.nav-link {
		color: var(--text-primary);
		text-decoration: none;
		padding: 0.5rem 1rem;
		border-radius: 0.25rem;
		transition: color 0.2s;
		cursor: pointer;
		background: none;
		border: none;
		font-size: 1rem;
	}

	.nav-link:hover {
		color: var(--accent);
	}

	.nav-link.active {
		color: var(--accent);
		font-weight: 600;
	}

	.nav-links {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}

	.ram-indicator {
		position: absolute;
		left: 50%;
		transform: translateX(-50%);
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.75rem;
		color: var(--text-secondary);
	}

	.ram-bar-container {
		width: 100px;
		height: 6px;
		background-color: var(--bg-tertiary);
		border-radius: 3px;
		overflow: hidden;
	}

	.ram-bar-fill {
		height: 100%;
		transition: width 0.3s ease, background-color 0.3s ease;
	}

	.ram-text {
		min-width: 100px;
		text-align: right;
	}
</style>
