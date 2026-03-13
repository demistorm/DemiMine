<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { servers, loadServers, updateServerStatus } from '$lib/stores/servers';
	import { api } from '$lib/api';
	import Console from '$lib/components/Console.svelte';
	import Files from '$lib/components/Files.svelte';
	import Settings from '$lib/components/Settings.svelte';
	import Backups from '$lib/components/Backups.svelte';
	import PluginBrowser from '$lib/components/PluginBrowser.svelte';

	let activeTab = 'console';
	let serverId = parseInt($page.params.id);
	let actionLoading = false;

	$: server = $servers?.find(s => s.id === serverId);

	onMount(() => {
		if (!$servers || $servers.length === 0) {
			loadServers();
		}
	});

	function goBack() {
		goto('/');
	}

	function setTab(tab: string) {
		activeTab = tab;
	}

	function getStatusColor(status: string) {
		switch (status) {
			case 'running': return 'var(--success)';
			case 'stopped': return 'var(--error)';
			case 'starting': return 'var(--warning)';
			default: return 'var(--text-secondary)';
		}
	}

	function getStatusText(status: string) {
		switch (status) {
			case 'running': return 'Online';
			case 'stopped': return 'Offline';
			case 'starting': return 'Starting';
			case 'stopping': return 'Stopping';
			default: return status;
		}
	}

	async function startServer() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/servers/${serverId}/start`);
			updateServerStatus(serverId, 'running');
		} catch (err) {
			console.error('Failed to start server:', err);
		} finally {
			actionLoading = false;
		}
	}

	async function stopServer() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/servers/${serverId}/stop`);
			updateServerStatus(serverId, 'stopped');
		} catch (err) {
			console.error('Failed to stop server:', err);
		} finally {
			actionLoading = false;
		}
	}

	async function restartServer() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/servers/${serverId}/restart`);
			updateServerStatus(serverId, 'running');
		} catch (err) {
			console.error('Failed to restart server:', err);
		} finally {
			actionLoading = false;
		}
	}

	function getLoadersForServer(): string[] {
		if (!server) return [];
		switch (server.type.toLowerCase()) {
			case 'paper': return ['paper'];
			case 'purpur': return ['purpur', 'paper'];
			default: return [];
		}
	}

	function supportsPlugins(): boolean {
		if (!server) return false;
		const type = server.type.toLowerCase();
		return type === 'paper' || type === 'purpur';
	}
</script>

<svelte:head>
	<title>{server?.name || 'Server'} - DemiMine</title>
</svelte:head>

<div class="server-detail">
	<div class="header">
		<button class="back-btn" on:click={goBack}>
			← Back to Canvas
		</button>
		<div class="server-info">
			<h1>{server?.name || 'Loading...'}</h1>
			{#if server}
				<span class="status">
					<span class="status-dot" style="background-color: {getStatusColor(server.status)}"></span>
					{getStatusText(server.status)}
				</span>
			{/if}
		</div>
		<div class="actions">
			{#if server}
				{#if server.status === 'running'}
					<button class="action-btn warning" on:click={restartServer} disabled={actionLoading}>
						Restart
					</button>
					<button class="action-btn danger" on:click={stopServer} disabled={actionLoading}>
						Stop
					</button>
				{:else}
					<button class="action-btn success" on:click={startServer} disabled={actionLoading}>
						Start
					</button>
				{/if}
			{/if}
		</div>
	</div>

	<div class="tabs">
		<button class="tab" class:active={activeTab === 'console'} on:click={() => setTab('console')}>
			Console
		</button>
		<button class="tab" class:active={activeTab === 'files'} on:click={() => setTab('files')}>
			Files
		</button>
		{#if supportsPlugins()}
			<button class="tab" class:active={activeTab === 'plugins'} on:click={() => setTab('plugins')}>
				Plugins
			</button>
		{/if}
		<button class="tab" class:active={activeTab === 'backups'} on:click={() => setTab('backups')}>
			Backups
		</button>
		<button class="tab" class:active={activeTab === 'settings'} on:click={() => setTab('settings')}>
			Settings
		</button>
	</div>

	<div class="content">
		{#if server}
			<div class="tab-content">
				<div class:hidden={activeTab !== 'console'}>
					<Console id={serverId} type="server" />
				</div>
				<div class:hidden={activeTab !== 'files'}>
					<Files id={serverId} type="server" />
				</div>
				<div class:hidden={activeTab !== 'plugins'}>
					<PluginBrowser 
						targetType="server" 
						targetId={serverId} 
						gameVersion={server.version}
						loaders={getLoadersForServer()}
					/>
				</div>
				<div class:hidden={activeTab !== 'backups'}>
					<Backups serverId={serverId} />
				</div>
				<div class:hidden={activeTab !== 'settings'}>
					<Settings {server} on:deleted={goBack} />
				</div>
			</div>
		{:else}
			<div class="loading">Loading server...</div>
		{/if}
	</div>
</div>

<style>
	.server-detail {
		position: fixed;
		top: 56px;
		left: 0;
		right: 0;
		bottom: 0;
		display: flex;
		flex-direction: column;
		background: var(--bg-primary);
	}

	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1.5rem;
		padding: 1rem 2rem;
		border-bottom: 1px solid var(--border);
	}

	.back-btn {
		background: transparent;
		color: var(--text-secondary);
		border: none;
		cursor: pointer;
		font-size: 1rem;
		padding: 0.5rem;
		transition: color 0.2s;
	}

	.back-btn:hover {
		color: var(--text-primary);
	}

	.server-info {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex: 1;
	}

	.server-info h1 {
		margin: 0;
		font-size: 1.25rem;
		color: var(--text-primary);
	}

	.status {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text-secondary);
		font-size: 0.875rem;
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.action-btn {
		padding: 0.5rem 1rem;
		border-radius: 0.375rem;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		border: 1px solid transparent;
		transition: all 0.2s;
	}

	.action-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.action-btn.success {
		background-color: var(--success);
		color: white;
	}

	.action-btn.success:hover:not(:disabled) {
		opacity: 0.9;
	}

	.action-btn.warning {
		background-color: var(--warning);
		color: #1a1a1a;
	}

	.action-btn.warning:hover:not(:disabled) {
		opacity: 0.9;
	}

	.action-btn.danger {
		background-color: transparent;
		color: var(--error);
		border-color: var(--error);
	}

	.action-btn.danger:hover:not(:disabled) {
		background-color: var(--error);
		color: white;
	}

	.tabs {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--border);
		padding: 0 2rem;
	}

	.tab {
		background: transparent;
		color: var(--text-secondary);
		border: none;
		padding: 1rem 1.5rem;
		cursor: pointer;
		font-size: 0.9375rem;
		font-weight: 500;
		border-bottom: 2px solid transparent;
		transition: all 0.2s;
	}

	.tab:hover {
		color: var(--text-primary);
	}

	.tab.active {
		color: var(--accent);
		border-bottom-color: var(--accent);
	}

	.content {
		flex: 1;
		overflow: auto;
		padding: 0;
	}

	.tab-content {
		height: 100%;
		width: 100%;
		display: flex;
	}

	.tab-content > div {
		flex: 1;
		height: 100%;
		width: 100%;
	}

	.loading {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		color: var(--text-secondary);
	}

	.tab-content > .hidden {
		display: none !important;
	}
</style>
