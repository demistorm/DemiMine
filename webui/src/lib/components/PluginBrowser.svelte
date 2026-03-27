<script lang="ts">
	import { onMount } from 'svelte';
	import { modrinthApi, pluginApi } from '$lib/api';
	import type { ModrinthProjectHit, InstalledPlugin, PluginUpdateResult } from '$lib/api';
	import { inputMode } from '$lib/stores/inputMode';

	export let targetType: 'server' | 'proxy';
	export let targetId: number;
	export let gameVersion: string;
	export let loaders: string[];

	let searchQuery = '';
	let searchResults: ModrinthProjectHit[] = [];
	let installedPlugins: InstalledPlugin[] = [];
	let updates: PluginUpdateResult[] = [];
	let searching = false;
	let installing: string | null = null;
	let activeView: 'browse' | 'installed' | 'updates' = 'browse';

	$: isInstalled = (projectId: string) => installedPlugins?.some(p => p.project_id === projectId) ?? false;

	async function loadInstalledPlugins() {
		try {
			const data = await pluginApi.getInstalled(targetType, targetId);
			installedPlugins = data ?? [];
		} catch (err) {
			console.error('Failed to load installed plugins:', err);
			installedPlugins = [];
		}
	}

	async function searchPlugins() {
		if (!searchQuery.trim()) {
			searchResults = [];
			return;
		}
		searching = true;
		try {
			const result = await modrinthApi.search({
				query: searchQuery,
				limit: 20,
				loaders,
				game_version: gameVersion
			});
			searchResults = result.hits;
		} catch (err) {
			console.error('Search failed:', err);
		} finally {
			searching = false;
		}
	}

	async function installPlugin(project: ModrinthProjectHit) {
		installing = project.project_id;
		try {
			const installed = await pluginApi.install(targetType, targetId, {
				project_id: project.project_id,
				game_version: gameVersion
			});
			installedPlugins = [...installedPlugins, installed];
		} catch (err) {
			console.error('Failed to install plugin:', err);
		} finally {
			installing = null;
		}
	}

	async function uninstallPlugin(projectId: string) {
		try {
			await pluginApi.uninstall(targetType, targetId, projectId);
			installedPlugins = installedPlugins.filter(p => p.project_id !== projectId);
		} catch (err) {
			console.error('Failed to uninstall plugin:', err);
		}
	}

	async function checkForUpdates() {
		try {
			const data = await pluginApi.checkUpdates(targetType, targetId);
			updates = data ?? [];
		} catch (err) {
			console.error('Failed to check updates:', err);
			updates = [];
		}
	}

	async function updatePlugin(projectId: string) {
		try {
			const updated = await pluginApi.update(targetType, targetId, projectId, gameVersion);
			const idx = installedPlugins.findIndex(p => p.project_id === projectId);
			if (idx !== -1) {
				installedPlugins[idx] = updated;
			}
			updates = updates.filter(u => u.plugin.project_id !== projectId);
		} catch (err) {
			console.error('Failed to update plugin:', err);
		}
	}

	function formatDownloads(count: number): string {
		if (count >= 1000000) return (count / 1000000).toFixed(1) + 'M';
		if (count >= 1000) return (count / 1000).toFixed(1) + 'K';
		return count.toString();
	}

	onMount(() => {
		loadInstalledPlugins();
		checkForUpdates();
	});
</script>

<div class="plugin-browser" class:mobile={$inputMode === 'mobile'}>
	<div class="header">
		<h2>Plugins</h2>
		<div class="view-tabs">
			<button class="view-tab" class:active={activeView === 'browse'} on:click={() => activeView = 'browse'}>
				Browse
			</button>
			<button class="view-tab" class:active={activeView === 'installed'} on:click={() => { activeView = 'installed'; loadInstalledPlugins(); }}>
				Installed ({installedPlugins.length})
			</button>
			<button class="view-tab" class:active={activeView === 'updates'} on:click={() => { activeView = 'updates'; checkForUpdates(); }}>
				{#if (updates || []).filter(u => u.has_update).length > 0}
					Updates ({(updates || []).filter(u => u.has_update).length})
				{:else}
					Updates
				{/if}
			</button>
		</div>
	</div>

	{#if activeView === 'browse'}
		<div class="search-section">
			<input 
				type="text" 
				bind:value={searchQuery}
				on:keydown={(e) => e.key === 'Enter' && searchPlugins()}
				placeholder="Search plugins..."
				class="search-input"
			/>
			<button class="btn btn-primary" on:click={searchPlugins} disabled={searching}>
				{#if searching}
					Searching...
				{:else}
					Search
				{/if}
			</button>
		</div>

		{#if searching}
			<div class="loading-state">
				<p>Searching...</p>
			</div>
		{:else if searchResults.length === 0 && searchQuery}
			<div class="empty-state">
				<p>No plugins found. Try a different search term.</p>
			</div>
		{:else if searchResults.length > 0}
			<div class="plugin-grid">
				{#each searchResults as project (project.project_id)}
					<div class="plugin-card">
						{#if project.icon_url}
							<img src={project.icon_url} alt={project.title} class="plugin-icon" />
						{:else}
							<div class="icon-placeholder">
								<svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
									<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/>
								</svg>
							</div>
						{/if}
						<div class="plugin-info">
							<h3>{project.title}</h3>
							<p class="description">{project.description}</p>
							<div class="meta">
								<span>{formatDownloads(project.downloads)} downloads</span>
								<span class="separator">•</span>
								<span>{(project.loaders || []).join(', ')}</span>
							</div>
						</div>
						<div class="plugin-actions">
							{#if isInstalled(project.project_id)}
								<button class="btn installed" disabled>Installed</button>
							{:else if installing === project.project_id}
								<button class="btn btn-primary" disabled>Installing...</button>
							{:else}
								<button class="btn btn-primary" on:click={() => installPlugin(project)}>Install</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}

	{:else if activeView === 'installed'}
		{#if installedPlugins.length === 0}
			<div class="empty-state">No plugins installed yet. Browse plugins to install some!</div>
		{:else}
			<div class="plugin-list">
				{#each installedPlugins as plugin (plugin.project_id)}
					<div class="plugin-item">
						<div class="plugin-info">
							<h3>{plugin.project_name}</h3>
							<div class="meta">
								<span>{plugin.version_number}</span>
								<span class="separator">•</span>
								<span>{plugin.filename}</span>
							</div>
						</div>
						<button class="btn btn-danger" on:click={() => uninstallPlugin(plugin.project_id)}>
							Uninstall
						</button>
					</div>
				{/each}
			</div>
		{/if}

	{:else if activeView === 'updates'}
		{#if (updates || []).filter(u => u.has_update).length === 0}
			<div class="empty-state">All plugins are up to date!</div>
		{:else}
			<div class="plugin-list">
				{#each (updates || []).filter(u => u.has_update) as update (update.plugin.project_id)}
					<div class="plugin-item update-available">
						<div class="plugin-info">
							<h3>{update.plugin.project_name}</h3>
							<div class="meta">
								<span>{update.current_version}</span>
								<span class="arrow">→</span>
								<span class="new-version">{update.latest_version}</span>
							</div>
						</div>
						<button class="btn btn-primary" on:click={() => updatePlugin(update.plugin.project_id)}>
							Update
						</button>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<style>
	.plugin-browser {
		padding: 1.5rem;
		max-width: 1200px;
	}

	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.header h2 {
		color: var(--text-primary);
		font-size: 1.5rem;
		margin: 0;
	}

	.view-tabs {
		display: flex;
		gap: 0.5rem;
	}

	.view-tab {
		padding: 0.5rem 1rem;
		background: var(--bg-secondary);
		border: 3px solid var(--border);
		border-radius: 0;
		color: var(--text-secondary);
		cursor: pointer;
		transition: all 0.2s;
	}

	.view-tab:hover {
		color: var(--text-primary);
	}

	.view-tab.active {
		background: var(--accent);
		border-color: var(--accent);
		color: white;
	}

	.search-section {
		display: flex;
		gap: 1rem;
		margin-bottom: 1.5rem;
	}

	.search-input {
		flex: 1;
		padding: 0.75rem 1rem;
		background: var(--bg-secondary);
		border: 3px solid var(--border);
		border-radius: 0;
		color: var(--text-primary);
		font-size: 0.9375rem;
	}

	.search-input::placeholder {
		color: var(--text-secondary);
	}

	.btn {
		padding: 0.5rem 1rem;
		border-radius: 0;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		border: 3px solid var(--border);
		background-color: var(--bg-tertiary);
		color: var(--text-primary);
	}

	.btn:hover {
		background-color: var(--bg-primary);
	}

	.btn-primary {
		background-color: var(--accent);
		border-color: var(--accent);
		color: white;
	}

	.btn-primary:hover {
		background-color: var(--accent-hover);
	}

	.btn-danger {
		color: var(--error);
		border-color: var(--error);
	}

	.btn-danger:hover {
		background-color: var(--error);
		color: white;
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn.installed {
		background-color: var(--success);
		border-color: var(--success);
		color: white;
		opacity: 0.7;
	}

	.plugin-grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
		gap: 1rem;
	}

	.plugin-card {
		display: flex;
		gap: 1rem;
		padding: 1rem;
		background: var(--bg-secondary);
		border: 3px solid var(--border);
		border-radius: 0;
		transition: border-color 0.2s;
	}

	.plugin-card:hover {
		border-color: var(--accent);
	}

	.plugin-icon {
		width: 48px;
		height: 48px;
		flex-shrink: 0;
		border-radius: 0;
		object-fit: cover;
	}

	.icon-placeholder {
		width: 48px;
		height: 48px;
		background: var(--bg-tertiary);
		border-radius: 0;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--text-secondary);
	}

	.plugin-info {
		flex: 1;
		min-width: 0;
	}

	.plugin-info h3 {
		color: var(--text-primary);
		font-size: 0.9375rem;
		margin: 0 0 0.25rem 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.plugin-info .description {
		color: var(--text-secondary);
		font-size: 0.8125rem;
		margin: 0 0 0.5rem 0;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.plugin-info .meta {
		color: var(--text-secondary);
		font-size: 0.75rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.separator {
		color: var(--border);
	}

	.plugin-actions {
		display: flex;
		align-items: center;
	}

	.plugin-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.plugin-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.25rem;
		background: var(--bg-secondary);
		border: 3px solid var(--border);
		border-radius: 0;
	}

	.plugin-item.update-available {
		border-color: var(--accent);
	}

	.plugin-item h3 {
		color: var(--text-primary);
		font-size: 0.9375rem;
		margin: 0 0 0.25rem 0;
	}

	.arrow {
		color: var(--accent);
	}

	.new-version {
		color: var(--success);
		font-weight: 500;
	}

	.loading-state, .empty-state {
		text-align: center;
		padding: 3rem;
		color: var(--text-secondary);
	}

	.mobile .plugin-browser {
		padding: 1rem;
	}

	.mobile .header {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.75rem;
	}

	.mobile .view-tabs {
		width: 100%;
		overflow-x: auto;
		scrollbar-width: none;
	}

	.mobile .view-tabs::-webkit-scrollbar {
		display: none;
	}

	.mobile .view-tab {
		flex-shrink: 0;
	}

	.mobile .search-section {
		flex-direction: column;
		gap: 0.5rem;
	}

	.mobile .plugin-grid {
		grid-template-columns: 1fr;
	}

	.mobile .plugin-item {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.75rem;
	}

	.mobile .plugin-item .btn {
		width: 100%;
		text-align: center;
	}
</style>
