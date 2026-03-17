<script lang="ts">
	import { onMount } from 'svelte';
	import { loadGlobalSettings } from '$lib/stores/settings';
	import { api } from '$lib/api';

	interface APIKey {
		id: number;
		name: string;
		key: string;
		created_at: string;
		last_used_at: string | null;
	}

	let apiKeys: APIKey[] = [];
	let loading = true;
	let newName = '';
	let creating = false;
	let successMessage = '';
	let errorMessage = '';

	onMount(async () => {
		await loadGlobalSettings();
		await loadAPIKeys();
	});

	async function loadAPIKeys() {
		try {
			apiKeys = await api.get<APIKey[]>('/api/api-keys');
		} catch (err: any) {
			console.error('Failed to load API keys:', err);
		} finally {
			loading = false;
		}
	}

	async function createAPIKey() {
		if (!newName.trim()) return;

		creating = true;
		errorMessage = '';
		successMessage = '';

		try {
			const result = await api.post<APIKey>('/api/api-keys', { name: newName });
			apiKeys = [result, ...apiKeys];
			newName = '';
			successMessage = 'API key created successfully!';
			setTimeout(() => successMessage = '', 3000);
		} catch (err: any) {
			errorMessage = 'Failed to create API key: ' + (err.message || 'Unknown error');
		} finally {
			creating = false;
		}
	}

	async function deleteAPIKey(id: number, name: string) {
		if (!confirm(`Are you sure you want to delete API key "${name}"?`)) return;

		try {
			await api['delete']<{ success: boolean }>(`/api/api-keys/${id}`);
			apiKeys = apiKeys.filter(k => k.id !== id);
			successMessage = 'API key deleted successfully!';
			setTimeout(() => successMessage = '', 3000);
		} catch (err: any) {
			errorMessage = 'Failed to delete API key: ' + (err.message || 'Unknown error');
		}
	}

	function copyToClipboard(key: string) {
		navigator.clipboard.writeText(key);
		successMessage = 'API key copied to clipboard!';
		setTimeout(() => successMessage = '', 3000);
	}
</script>

<svelte:head>
	<title>API Keys - DemiMine</title>
</svelte:head>

<main class="settings-container">
	{#if loading}
		<p class="loading">Loading API keys...</p>
	{:else}
		<div class="section">
			<h2>API Keys</h2>
			<p class="hint">Generate API keys for external integrations like DemiDynamic plugin.</p>

			{#if successMessage}
				<div class="alert success">
					<span>{successMessage}</span>
				</div>
			{/if}

			{#if errorMessage}
				<div class="alert error">
					<span>{errorMessage}</span>
				</div>
			{/if}

			{#if apiKeys.length === 0}
				<p class="empty-state">No API keys yet. Generate one to get started.</p>
			{/if}

			<div class="api-keys-list">
				{#each apiKeys as key (key.id)}
					<div class="api-key-item">
						<div class="api-key-info">
							<div class="api-key-name">{key.name}</div>
							<div class="api-key-details">
								<span class="api-key-created">Created: {new Date(key.created_at).toLocaleString()}</span>
								{#if key.last_used_at}
									<span class="api-key-used">Last used: {new Date(key.last_used_at).toLocaleString()}</span>
								{:else}
									<span class="api-key-unused">Never used</span>
								{/if}
							</div>
						</div>
						<div class="api-key-actions">
							<button class="btn btn-sm" on:click={() => copyToClipboard(key.key)}>📋 Copy Key</button>
							<button class="btn btn-sm btn-danger" on:click={() => deleteAPIKey(key.id, key.name)}>🗑 Delete</button>
						</div>
					</div>
				{/each}
			</div>

			<div class="create-key-section">
				<h3>Create New API Key</h3>
				<div class="field">
					<label for="keyName">Key Name</label>
					<input
						id="keyName"
						type="text"
						bind:value={newName}
						placeholder="e.g., DemiDynamic Plugin"
						disabled={creating}
					/>
					<div class="hint">Choose a descriptive name to help identify this key later.</div>
				</div>
				<button class="btn primary" on:click={createAPIKey} disabled={creating || !newName.trim()}>
					{creating ? 'Creating...' : 'Generate API Key'}
				</button>
			</div>
		</div>
	{/if}
</main>

<style>
	.settings-container {
		max-width: 900px;
		margin: 80px auto;
		padding-bottom: 80px;
	}

	.loading {
		text-align: center;
		padding: 2rem;
		color: var(--text-secondary);
	}

	.section {
		margin-bottom: 2.5rem;
		padding: 1.5rem;
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 0.5rem;
	}

	.section h2 {
		color: var(--text-primary);
		font-size: 1.25rem;
		margin: 0 0 1.5rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--border);
	}

	.section h3 {
		color: var(--text-primary);
		font-size: 1.125rem;
		margin: 2rem 0 1rem;
	}

	.hint {
		display: block;
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-top: -0.5rem;
		margin-bottom: 1.5rem;
		opacity: 0.7;
	}

	.empty-state {
		text-align: center;
		padding: 2rem;
		color: var(--text-secondary);
		font-style: italic;
	}

	.alert {
		padding: 0.75rem 1rem;
		border-radius: 0.375rem;
		margin-bottom: 1.5rem;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.alert.success {
		background: var(--success);
		color: white;
	}

	.alert.error {
		background: var(--error);
		color: white;
	}

	.api-keys-list {
		margin-bottom: 2rem;
	}

	.api-key-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem;
		background: var(--bg-tertiary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		margin-bottom: 0.75rem;
	}

	.api-key-info {
		flex: 1;
	}

	.api-key-name {
		font-weight: 600;
		color: var(--text-primary);
		font-size: 1rem;
		margin-bottom: 0.375rem;
	}

	.api-key-details {
		display: flex;
		gap: 1rem;
		font-size: 0.875rem;
		color: var(--text-secondary);
	}

	.api-key-unused {
		color: var(--text-tertiary);
		font-style: italic;
	}

	.api-key-actions {
		display: flex;
		gap: 0.5rem;
	}

	.create-key-section {
		background: var(--bg-tertiary);
		padding: 1.5rem;
		border-radius: 0.375rem;
		border: 1px solid var(--border);
	}

	.field {
		margin-bottom: 1.5rem;
	}

	.field label {
		display: block;
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-bottom: 0.5rem;
		font-weight: 500;
	}

	.field input[type="text"] {
		width: 100%;
		max-width: 400px;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		font-size: 0.9375rem;
	}

	.field input:focus:not(:disabled) {
		outline: none;
		border-color: var(--accent);
	}

	.btn {
		padding: 0.625rem 1.25rem;
		border-radius: 0.375rem;
		font-size: 0.9375rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		border: 1px solid var(--border);
		background-color: var(--bg-secondary);
		color: var(--text-primary);
	}

	.btn:hover:not(:disabled) {
		background-color: var(--bg-tertiary);
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn.primary {
		background-color: var(--accent);
		border-color: var(--accent);
		color: white;
	}

	.btn.primary:hover:not(:disabled) {
		background-color: var(--accent-hover);
	}

	.btn-sm {
		padding: 0.375rem 0.75rem;
		font-size: 0.875rem;
	}

	.btn-danger {
		background-color: var(--error);
		border-color: var(--error);
		color: white;
	}

	.btn-danger:hover {
		background-color: #e53935;
	}
</style>
