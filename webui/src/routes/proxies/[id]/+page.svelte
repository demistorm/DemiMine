<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { proxies, loadProxies, updateProxyStatus } from '$lib/stores/servers';
	import { ws } from '$lib/websocket';
	import { api } from '$lib/api';
	import { inputMode } from '$lib/stores/inputMode';
	import Console from '$lib/components/Console.svelte';
	import Files from '$lib/components/Files.svelte';
	import ProxySettings from '$lib/components/ProxySettings.svelte';
	import PluginBrowser from '$lib/components/PluginBrowser.svelte';
	import ProfileButton from '$lib/components/ProfileButton.svelte';

	let activeTab = 'console';
	let proxyId = parseInt($page.params.id);
	let actionLoading = false;
	let wsChannel = `proxy:${proxyId}`;

	$: proxy = $proxies?.find(p => p.id === proxyId);
	
	onMount(async () => {
		if (!$proxies || $proxies.length === 0) {
			await loadProxies();
		}

		try {
			await ws.connect();
			ws.subscribe(wsChannel);
		} catch (err) {
			console.error('Failed to connect to WebSocket:', err);
		}
	});

	onDestroy(() => {
		ws.unsubscribe(wsChannel);
	});

	onMount(() => {
		if (!$proxies || $proxies.length === 0) {
			loadProxies();
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

	async function startProxy() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/proxies/${proxyId}/start`);
			updateProxyStatus(proxyId, 'running');
		} catch (err) {
			console.error('Failed to start proxy:', err);
		} finally {
			actionLoading = false;
		}
	}

	async function stopProxy() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/proxies/${proxyId}/stop`);
			updateProxyStatus(proxyId, 'stopped');
		} catch (err) {
			console.error('Failed to stop proxy:', err);
		} finally {
			actionLoading = false;
		}
	}

	async function restartProxy() {
		if (actionLoading) return;
		actionLoading = true;
		try {
			await api.post(`/api/proxies/${proxyId}/restart`);
			updateProxyStatus(proxyId, 'running');
		} catch (err) {
			console.error('Failed to restart proxy:', err);
		} finally {
			actionLoading = false;
		}
	}
</script>

<svelte:head>
	<title>{proxy?.name || 'Proxy'} - DemiMine</title>
</svelte:head>

<div class="proxy-detail" class:mobile={$inputMode === 'mobile'}>
	<div class="header">
		{#if $inputMode === 'mobile'}
			<div class="header-row-1">
				<button class="back-btn" on:click={goBack}>
					← Back
				</button>
				{#if proxy}
					<span class="status">
						<span class="status-dot" style="background-color: {getStatusColor(proxy.status)}"></span>
						{getStatusText(proxy.status)}
					</span>
				{/if}
				<div class="actions">
					{#if proxy}
						<ProfileButton targetId={proxyId} targetType="proxy" isRunning={proxy.status === 'running'} />
						{#if proxy.status === 'running'}
							<button class="action-btn warning" on:click={restartProxy} disabled={actionLoading}>
								Restart
							</button>
							<button class="action-btn danger" on:click={stopProxy} disabled={actionLoading}>
								Stop
							</button>
						{:else}
							<button class="action-btn success" on:click={startProxy} disabled={actionLoading}>
								Start
							</button>
						{/if}
					{/if}
				</div>
			</div>
			<div class="proxy-name-row">
				<h1>{proxy?.name || 'Loading...'}</h1>
			</div>
		{:else}
			<button class="back-btn" on:click={goBack}>
				← Back
			</button>
			<div class="proxy-info">
				<h1>{proxy?.name || 'Loading...'}</h1>
				{#if proxy}
					<span class="status">
						<span class="status-dot" style="background-color: {getStatusColor(proxy.status)}"></span>
						{getStatusText(proxy.status)}
					</span>
				{/if}
			</div>
			<div class="actions">
				{#if proxy}
					<ProfileButton targetId={proxyId} targetType="proxy" isRunning={proxy.status === 'running'} />
					{#if proxy.status === 'running'}
						<button class="action-btn warning" on:click={restartProxy} disabled={actionLoading}>
							Restart
						</button>
						<button class="action-btn danger" on:click={stopProxy} disabled={actionLoading}>
							Stop
						</button>
					{:else}
						<button class="action-btn success" on:click={startProxy} disabled={actionLoading}>
							Start
						</button>
					{/if}
				{/if}
			</div>
		{/if}
	</div>

	<div class="tabs">
		<button class="tab" class:active={activeTab === 'console'} on:click={() => setTab('console')}>
			Console
		</button>
		<button class="tab" class:active={activeTab === 'files'} on:click={() => setTab('files')}>
			Files
		</button>
		<button class="tab" class:active={activeTab === 'plugins'} on:click={() => setTab('plugins')}>
			Plugins
		</button>
		<button class="tab" class:active={activeTab === 'settings'} on:click={() => setTab('settings')}>
			Settings
		</button>
	</div>

	<div class="content">
		{#if proxy}
			<div class="tab-content">
				<div class:hidden={activeTab !== 'console'}>
					<Console id={proxyId} type="proxy" />
				</div>
				<div class:hidden={activeTab !== 'files'}>
					<Files id={proxyId} type="proxy" />
				</div>
				<div class:hidden={activeTab !== 'plugins'}>
					<PluginBrowser
						targetType="proxy"
						targetId={proxyId}
						gameVersion=""
						loaders={['velocity']}
					/>
				</div>
				<div class:hidden={activeTab !== 'settings'}>
					<ProxySettings {proxy} on:deleted={goBack} />
				</div>
			</div>
		{:else}
			<div class="loading">Loading proxy...</div>
		{/if}
	</div>
</div>

<style>
	.proxy-detail {
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
		border-bottom: 3px solid var(--border);
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

	.proxy-info {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex: 1;
	}

	.proxy-info h1 {
		margin: 0;
		font-size: 1.25rem;
		color: var(--text-primary);
		font-family: 'Minecraft', sans-serif;
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
		border-radius: 0;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		border: 3px solid transparent;
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
		border-bottom: 3px solid var(--border);
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
		border-bottom: 6px solid transparent;
		transition: all 0.2s;
		white-space: nowrap;
		font-family: 'Minecraft', sans-serif;
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

	.mobile .header {
		flex-direction: column;
		align-items: stretch;
		gap: 0;
		padding: 0.5rem 0.75rem;
	}

	.mobile .header-row-1 {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.mobile .header-row-1 .back-btn {
		padding: 0.25rem 0.5rem;
		font-size: 0.875rem;
		flex-shrink: 0;
	}

	.mobile .header-row-1 .status {
		flex-shrink: 0;
	}

	.mobile .header-row-1 .status .status-dot {
		width: 6px;
		height: 6px;
	}

	.mobile .header-row-1 .status {
		font-size: 0.75rem;
	}

	.mobile .header-row-1 .actions {
		margin-left: auto;
		gap: 0.3rem;
	}

	.mobile .header-row-1 .action-btn {
		padding: 0.25rem 0.5rem;
		font-size: 0.7rem;
		border-width: 1px;
	}

	.mobile .header-row-1 :global(.profile-btn) {
		padding: 0.25rem 0.5rem;
		font-size: 0.7rem;
		border-width: 1px;
	}

	.mobile .proxy-name-row {
		padding: 0.25rem 0;
	}

	.mobile .proxy-name-row h1 {
		margin: 0;
		font-size: 1rem;
		color: var(--text-primary);
	}

	.mobile .tabs {
		padding: 0;
		overflow-x: auto;
		scrollbar-width: none;
		-ms-overflow-style: none;
	}

	.mobile .tabs::-webkit-scrollbar {
		display: none;
	}

	.mobile .tab {
		padding: 0.75rem 1rem;
		font-size: 0.875rem;
	}
</style>
