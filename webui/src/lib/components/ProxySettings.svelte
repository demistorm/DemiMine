<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api, type Proxy } from '$lib/api';
	import { proxies } from '$lib/stores/servers';

	export let proxy: Proxy;

	let name = proxy.name || '';
	let ramMB = proxy.ram_mb || 512;
	let saving = false;
	let error = '';
	let success = '';
	let showDeleteConfirm = false;

	const dispatch = createEventDispatcher();

	async function saveChanges() {
		saving = true;
		error = '';
		success = '';

		try {
			await api.patch(`/api/proxies/${proxy.id}`, {
				name: name !== proxy.name ? name : undefined,
				ram_mb: ramMB !== proxy.ram_mb ? ramMB : undefined
			});
			
			success = 'Settings saved successfully';
			proxies.update(list => 
				list.map(p => p.id === proxy.id ? { 
					...p, 
					name,
					ram_mb: ramMB
				} : p)
			);
			
			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function deleteProxy() {
		if (!showDeleteConfirm) {
			showDeleteConfirm = true;
			return;
		}

		saving = true;
		try {
			await api.delete(`/api/proxies/${proxy.id}`);
			dispatch('deleted');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete proxy';
			showDeleteConfirm = false;
		} finally {
			saving = false;
		}
	}

	function cancelDelete() {
		showDeleteConfirm = false;
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		success = 'Copied to clipboard';
		setTimeout(() => success = '', 2000);
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
			<label for="name">Proxy Name</label>
			<input type="text" id="name" bind:value={name} />
		</div>

		<div class="field">
			<label for="ram">Memory (MB)</label>
			<input type="number" id="ram" bind:value={ramMB} min="256" step="256" />
			<span class="hint">Java heap size for the proxy (e.g., 512, 1024, 2048)</span>
		</div>
	</div>

	<div class="section">
		<h2>Network</h2>
		
		<div class="field">
			<label for="port">Host Port</label>
			<input type="number" id="port" value={proxy.host_port} disabled />
			<span class="hint">Port exposed on the host for player connections</span>
		</div>

		<div class="field">
			<label for="secret">Forwarding Secret</label>
			<div class="secret-field">
				<input type="text" id="secret" value={proxy.forwarding_secret} disabled />
				<button class="copy-btn" on:click={() => copyToClipboard(proxy.forwarding_secret)}>Copy</button>
			</div>
			<span class="hint">Used for Velocity modern forwarding. Configure backend servers with this secret.</span>
		</div>
	</div>

	<div class="section">
		<h2>Connected Servers</h2>
		
		{#if proxy.connected_servers && proxy.connected_servers.length > 0}
			<div class="server-list">
				{#each proxy.connected_servers as serverName}
					<div class="server-item">{serverName}</div>
				{/each}
			</div>
		{:else}
			<p class="no-servers">No servers connected to this proxy.</p>
		{/if}
	</div>

	<div class="section danger">
		<h2>Danger Zone</h2>
		
		<div class="danger-actions">
			{#if showDeleteConfirm}
				<div class="confirm-delete">
					<p>Are you sure? This will delete all proxy files and cannot be undone.</p>
					<div class="confirm-buttons">
						<button class="btn danger" on:click={deleteProxy} disabled={saving}>
							Yes, Delete Proxy
						</button>
						<button class="btn" on:click={cancelDelete}>
							Cancel
						</button>
					</div>
				</div>
			{:else}
				<button class="btn danger" on:click={deleteProxy}>
					Delete Proxy
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

	.secret-field {
		display: flex;
		gap: 0.5rem;
		max-width: 400px;
	}

	.secret-field input {
		flex: 1;
	}

	.copy-btn {
		padding: 0.625rem 1rem;
		background-color: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		cursor: pointer;
		font-size: 0.875rem;
	}

	.copy-btn:hover {
		background-color: var(--accent);
	}

	.server-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.server-item {
		padding: 0.5rem 0.75rem;
		background-color: var(--bg-tertiary);
		border-radius: 0.25rem;
		color: var(--text-primary);
		font-size: 0.875rem;
	}

	.no-servers {
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin: 0;
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
