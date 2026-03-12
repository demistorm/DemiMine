<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api, type Server } from '$lib/api';
	import { servers } from '$lib/stores/servers';

	export let server: Server;

	let ram = server.ram_mb || 2048;
	let autoShutdown = server.auto_shutdown_minutes || 15;
	let backupInterval = server.backup_interval_days || 7;
	let name = server.name || '';
	let saving = false;
	let error = '';
	let success = '';
	let deleteStep = 0;

	const dispatch = createEventDispatcher();

	async function saveChanges() {
		saving = true;
		error = '';
		success = '';

		try {
			await api.patch(`/api/servers/${server.id}`, {
				name: name !== server.name ? name : undefined,
				ram_mb: ram !== server.ram_mb ? ram : undefined,
				auto_shutdown_minutes: autoShutdown !== server.auto_shutdown_minutes ? autoShutdown : undefined,
				backup_interval_days: backupInterval !== server.backup_interval_days ? backupInterval : undefined
			});
			
			success = 'Settings saved successfully';
			servers.update(list => 
				list.map(s => s.id === server.id ? { 
					...s, 
					name,
					ram_mb: ram,
					auto_shutdown_minutes: autoShutdown,
					backup_interval_days: backupInterval
				} : s)
			);
			
			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function deleteServer() {
		if (deleteStep === 0) {
			deleteStep = 1;
			return;
		}
		if (deleteStep === 1) {
			deleteStep = 2;
			return;
		}

		saving = true;
		try {
			await api.delete(`/api/servers/${server.id}`);
			dispatch('deleted');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete server';
			deleteStep = 0;
		} finally {
			saving = false;
		}
	}

	function cancelDelete() {
		deleteStep = 0;
	}

	function formatBytes(mb: number): string {
		if (mb >= 1024) {
			return `${(mb / 1024).toFixed(1)} GB`;
		}
		return `${mb} MB`;
	}
</script>

<div class="settings">
	{#if error}
		<div class="alert error">{error}<button on:click={() => error = ''}>×</button></div>
	{/if}
	{#if success}
		<div class="alert success">{success}</div>
	{/if}

	<div class="section">
		<h2>General Settings</h2>
		
		<div class="field">
			<label for="name">Server Name</label>
			<input type="text" id="name" bind:value={name} />
		</div>

		<div class="field">
			<label for="type">Server Type</label>
			<input type="text" id="type" value={server.type} disabled />
			<span class="hint">Server type cannot be changed after creation</span>
		</div>

		<div class="field">
			<label for="version">Minecraft Version</label>
			<input type="text" id="version" value={server.version} disabled />
			<span class="hint">Version cannot be changed after creation</span>
		</div>
	</div>

	<div class="section">
		<h2>Resource Allocation</h2>
		
		<div class="field">
			<label for="ram">RAM Allocation</label>
			<div class="slider-container">
				<input 
					type="range" 
					id="ram" 
					bind:value={ram}
					min={512}
					max={16384}
					step={512}
				/>
				<span class="slider-value">{formatBytes(ram)}</span>
			</div>
		</div>
	</div>

	<div class="section">
		<h2>Automation</h2>
		
		<div class="field">
			<label for="shutdown">Auto-shutdown (minutes of inactivity)</label>
			<input type="number" id="shutdown" bind:value={autoShutdown} min={0} />
			<span class="hint">Set to 0 to disable auto-shutdown</span>
		</div>

		<div class="field">
			<label for="backup">Backup Interval (days)</label>
			<input type="number" id="backup" bind:value={backupInterval} min={0} />
			<span class="hint">Set to 0 to disable automatic backups</span>
		</div>
	</div>

	<div class="section">
		<h2>Network</h2>
		
		<div class="field">
			<label for="port">Host Port</label>
			<input type="number" id="port" value={server.host_port || 'Not exposed'} disabled />
			<span class="hint">
				{#if server.host_port}
					Server is accessible on port {server.host_port}
				{:else}
					Server is behind a proxy (not directly exposed)
				{/if}
			</span>
		</div>

		<div class="field">
			<label for="proxy">Proxy Assignment</label>
			<input 
				type="text" 
				id="proxy" 
				value={server.proxy_name || 'Standalone (no proxy)'} 
				disabled 
			/>
		</div>
	</div>

	<div class="section danger">
		<h2>Danger Zone</h2>
		
		<div class="danger-actions">
			{#if deleteStep === 2}
				<div class="confirm-delete">
					<p class="final-warning">FINAL WARNING: All server files and backups will be permanently deleted!</p>
					<div class="confirm-buttons">
						<button class="btn danger" on:click={deleteServer} disabled={saving}>
							Delete Permanently
						</button>
						<button class="btn" on:click={cancelDelete}>
							Cancel
						</button>
					</div>
				</div>
			{:else if deleteStep === 1}
				<div class="confirm-delete">
					<p>Are you sure? This will delete all server files and cannot be undone.</p>
					<div class="confirm-buttons">
						<button class="btn danger" on:click={deleteServer}>
							Yes, Continue
						</button>
						<button class="btn" on:click={cancelDelete}>
							Cancel
						</button>
					</div>
				</div>
			{:else}
				<button class="btn danger" on:click={deleteServer}>
					Delete Server
				</button>
			{/if}
		</div>
	</div>

	<div class="save-bar">
		<button class="btn primary" on:click={saveChanges} disabled={saving}>
			{saving ? 'Saving...' : 'Save Changes'}
		</button>
	</div>
</div>

<style>
	.settings {
		max-width: 800px;
		padding-bottom: 80px;
	}

	.alert {
		padding: 0.75rem 1rem;
		border-radius: 0.375rem;
		margin-bottom: 1.5rem;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.alert.error {
		background: var(--error);
		color: white;
	}

	.alert.success {
		background: var(--success);
		color: white;
	}

	.alert button {
		background: transparent;
		border: none;
		color: white;
		font-size: 1.25rem;
		cursor: pointer;
		padding: 0 0.5rem;
	}

	.section {
		margin-bottom: 2.5rem;
		padding: 1.5rem;
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 0.5rem;
	}

	.section.danger {
		border-color: var(--error);
		background: rgba(239, 68, 68, 0.05);
	}

	.section h2 {
		color: var(--text-primary);
		font-size: 1.125rem;
		margin: 0 0 1.5rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--border);
	}

	.field {
		margin-bottom: 1.5rem;
	}

	.field:last-child {
		margin-bottom: 0;
	}

	.field label {
		display: block;
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-bottom: 0.5rem;
		font-weight: 500;
	}

	.field input[type="text"],
	.field input[type="number"] {
		width: 100%;
		max-width: 400px;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		font-size: 0.9375rem;
	}

	.field input:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.field input:focus:not(:disabled) {
		outline: none;
		border-color: var(--accent);
	}

	.hint {
		display: block;
		color: var(--text-secondary);
		font-size: 0.75rem;
		margin-top: 0.375rem;
		opacity: 0.7;
	}

	.slider-container {
		display: flex;
		align-items: center;
		gap: 1rem;
		max-width: 400px;
	}

	.slider-container input[type="range"] {
		flex: 1;
		height: 6px;
		background: var(--bg-tertiary);
		border-radius: 3px;
		-webkit-appearance: none;
	}

	.slider-container input[type="range"]::-webkit-slider-thumb {
		-webkit-appearance: none;
		width: 18px;
		height: 18px;
		background: var(--accent);
		border-radius: 50%;
		cursor: pointer;
	}

	.slider-value {
		min-width: 80px;
		color: var(--text-primary);
		font-weight: 500;
	}

	.danger-actions {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.confirm-delete p {
		color: var(--text-secondary);
		margin: 0 0 1rem;
	}

	.confirm-delete p.final-warning {
		color: #f87171;
		font-weight: 600;
	}

	.confirm-buttons {
		display: flex;
		gap: 0.75rem;
	}

	.btn {
		padding: 0.625rem 1.25rem;
		border-radius: 0.375rem;
		font-size: 0.9375rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		border: 1px solid var(--border);
		background-color: var(--bg-tertiary);
		color: var(--text-primary);
	}

	.btn:hover:not(:disabled) {
		background-color: var(--bg-secondary);
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn.primary {
		background-color: var(--accent);
		border-color: var(--accent);
	}

	.btn.primary:hover:not(:disabled) {
		background-color: var(--accent-hover);
	}

	.btn.danger {
		color: var(--error);
		border-color: var(--error);
		background-color: transparent;
	}

	.btn.danger:hover:not(:disabled) {
		background-color: var(--error);
		color: white;
	}

	.save-bar {
		position: fixed;
		bottom: 0;
		left: 200px;
		right: 0;
		padding: 1rem 2rem;
		background-color: var(--bg-secondary);
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: flex-end;
		z-index: 10;
	}
</style>
