<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { browser } from '$app/environment';
	import '../app.css';
	import { page } from '$app/stores';
	import { token } from '$lib/stores/auth';
	import { resourcesApi, type SystemResources, getToken } from '$lib/api';
	import { backupStatus, isBackupRunning, backupOperationLabel } from '$lib/stores/servers';
	import { ws } from '$lib/websocket';
	import { inputMode } from '$lib/stores/inputMode';
	import { settingsApi } from '$lib/api';

	if (browser) {
		const saved = localStorage.getItem('accent_color');
		if (saved) {
			document.documentElement.style.setProperty('--accent', saved);
			const r = parseInt(saved.slice(1, 3), 16);
			const g = parseInt(saved.slice(3, 5), 16);
			const b = parseInt(saved.slice(5, 7), 16);
			const dr = Math.max(0, Math.round(r * 0.75)).toString(16).padStart(2, '0');
			const dg = Math.max(0, Math.round(g * 0.75)).toString(16).padStart(2, '0');
			const db = Math.max(0, Math.round(b * 0.75)).toString(16).padStart(2, '0');
			document.documentElement.style.setProperty('--accent-hover', `#${dr}${dg}${db}`);
		}
	}

	let usedMB = 0;
	let maxMB = 0;
	let pollingInterval: number;
	let backupTimeout: number | null = null;
	let wsSetup = false;
	let mobileMenuOpen = false;

	async function fetchResources() {
		try {
			const resources = await resourcesApi.get();
			usedMB = resources.used_mb;
			maxMB = resources.max_mb;
		} catch (e) {
			console.error('Failed to fetch resources:', e);
		}
	}

	function setupBackupStatusHandler() {
		ws.on('backup_status', (message: any) => {
			backupStatus.set({
				status: message.status,
				backupId: message.backup_id || null,
				message: message.message || '',
				timestamp: message.timestamp || null
			});

			if (message.status === 'complete' || message.status === 'failed' || message.status === 'restoring_complete') {
				if (backupTimeout) clearTimeout(backupTimeout);
				backupTimeout = setTimeout(() => {
					backupStatus.set({ status: 'idle', backupId: null, message: '', timestamp: null });
				}, 5000) as unknown as number;
			}
		});
	}

	async function connectWebSocket() {
		if (wsSetup) return;
		const authToken = getToken();
		if (!authToken) return;

		try {
			ws.disconnect();
			await ws.connect();
			setupBackupStatusHandler();
			wsSetup = true;
		} catch (e) {
			console.error('WebSocket connection failed:', e);
		}
	}

	onMount(async () => {
		fetchResources();
		pollingInterval = setInterval(fetchResources, 2000) as unknown as number;
		await connectWebSocket();
		if ($token) {
		try {
			const settings = await settingsApi.get();
			const color = settings.accent_color || '#8b5e2a';
			document.documentElement.style.setProperty('--accent', color);
			const r = parseInt(color.slice(1, 3), 16);
			const g = parseInt(color.slice(3, 5), 16);
			const b = parseInt(color.slice(5, 7), 16);
			const dr = Math.max(0, Math.round(r * 0.75)).toString(16).padStart(2, '0');
			const dg = Math.max(0, Math.round(g * 0.75)).toString(16).padStart(2, '0');
			const db = Math.max(0, Math.round(b * 0.75)).toString(16).padStart(2, '0');
			document.documentElement.style.setProperty('--accent-hover', `#${dr}${dg}${db}`);
			localStorage.setItem('accent_color', color);
		} catch (e) {}
		}
	});

	$: if ($token && !wsSetup) {
		wsSetup = false;
		connectWebSocket();
	}

	$: if (!$token) {
		wsSetup = false;
		ws.disconnect();
	}

	onDestroy(() => {
		if (pollingInterval) clearInterval(pollingInterval);
		if (backupTimeout) clearTimeout(backupTimeout);
	});

	function handleLogout() {
		token.set(null);
	}

	function toggleMobileMenu() {
		mobileMenuOpen = !mobileMenuOpen;
	}

	function formatMB(mb: number): string {
		if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
		return `${mb}MB`;
	}

	$: percentage = maxMB > 0 ? (usedMB / maxMB) * 100 : 0;
	$: barColor = percentage > 95 ? 'var(--error)' : percentage > 80 ? 'var(--warning)' : 'var(--accent)';
	$: ramText = `${formatMB(usedMB)}/${formatMB(maxMB)}`;
</script>

{#if $page.url.pathname !== '/login'}
	<nav class="top-nav">
		<div class="nav-brand">
			{#if $isBackupRunning}
				<div class="backup-indicator" title={$backupStatus.message}>
					<span class="backup-spinner"></span>
					<span class="backup-label">{$backupOperationLabel}</span>
				</div>
			{/if}
			<a href="/" class="nav-link">
				<strong>DemiMine</strong>
			</a>
		</div>

		{#if $inputMode === 'desktop'}
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
		{:else}
			<span class="ram-compact" style="color: {barColor};">{ramText}</span>

			<button class="hamburger-btn" on:click={toggleMobileMenu}>
				<span class="hamburger-line"></span>
				<span class="hamburger-line"></span>
				<span class="hamburger-line"></span>
			</button>

			{#if mobileMenuOpen}
				<div class="mobile-menu-overlay" on:click={toggleMobileMenu}>
					<div class="mobile-menu" on:click|stopPropagation>
						<a href="/settings" class="mobile-nav-link" class:active={$page.url.pathname === '/settings'} on:click={toggleMobileMenu}>
							Settings
						</a>
						<a href="/" class="mobile-nav-link" class:active={$page.url.pathname === '/'} on:click={toggleMobileMenu}>
							Servers
						</a>
						{#if $token}
							<button class="mobile-nav-link" on:click={() => { handleLogout(); toggleMobileMenu(); }}>
								Logout
							</button>
						{:else}
							<a href="/login" class="mobile-nav-link" on:click={toggleMobileMenu}>
								Login
							</a>
						{/if}
					</div>
				</div>
			{/if}
		{/if}
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
		border-radius: 0;
		transition: color 0.2s;
		cursor: pointer;
		background: none;
		border: none;
		font-size: 1rem;
		font-family: 'Minecraft', sans-serif;
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
		border-radius: 0;
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

	.ram-compact {
		font-size: 0.75rem;
		font-weight: 500;
		white-space: nowrap;
	}

	.backup-indicator {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.2rem 0.6rem;
		background: rgba(59, 130, 246, 0.15);
		border: 2px solid var(--accent);
		margin-right: 0.75rem;
	}

	.backup-spinner {
		width: 12px;
		height: 12px;
		border: 2px solid var(--accent);
		border-top-color: transparent;
		border-radius: 50%;
		animation: spin 1s linear infinite;
	}

	.backup-label {
		font-size: 0.7rem;
		color: var(--accent);
		font-weight: 500;
		white-space: nowrap;
	}

	.hamburger-btn {
		display: flex;
		flex-direction: column;
		justify-content: space-around;
		width: 28px;
		height: 24px;
		background: transparent;
		border: none;
		cursor: pointer;
		padding: 0;
		gap: 4px;
	}

	.hamburger-line {
		display: block;
		width: 100%;
		height: 3px;
		background-color: var(--text-primary);
		transition: background-color 0.2s;
	}

	.hamburger-btn:hover .hamburger-line {
		background-color: var(--accent);
	}

	.mobile-menu-overlay {
		position: fixed;
		top: 56px;
		left: 0;
		right: 0;
		background: rgba(0, 0, 0, 0.7);
		z-index: 99;
	}

	.mobile-menu {
		background: var(--bg-secondary);
		border-bottom: 3px solid var(--border);
		padding: 0.5rem 0;
	}

	.mobile-nav-link {
		display: block;
		color: var(--text-primary);
		text-decoration: none;
		padding: 0.75rem 1.5rem;
		border-radius: 0;
		transition: all 0.2s;
		cursor: pointer;
		background: none;
		border: none;
		font-size: 1rem;
		text-align: left;
		width: 100%;
	}

	.mobile-nav-link:hover,
	.mobile-nav-link.active {
		color: var(--accent);
		background: var(--bg-tertiary);
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}
</style>
