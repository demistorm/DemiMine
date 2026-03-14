<script lang="ts">
	import { onMount } from 'svelte';
	import { globalSettings, proxyMCVersion, loadGlobalSettings, saveGlobalSettings } from '$lib/stores/settings';

	let saving = false;
	let error = '';
	let success = '';
	let localProxyMCVersion = '';

	$: localProxyMCVersion = $proxyMCVersion;

	function handleVersionInput(e: Event) {
		localProxyMCVersion = (e.target as HTMLInputElement).value;
	}

	async function saveChanges() {
		saving = true;
		error = '';
		success = '';

		try {
			await saveGlobalSettings({ proxy_mc_version: localProxyMCVersion });
			success = 'Settings saved successfully';
			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	onMount(() => {
		loadGlobalSettings();
	});
</script>

<svelte:head>
	<title>Settings - DemiMine</title>
</svelte:head>

<main class="settings-container">
	{#if error}
		<div class="alert error">{error}<button on:click={() => error = ''}>×</button></div>
	{/if}
	{#if success}
		<div class="alert success">{success}</div>
	{/if}

	<div class="section">
		<h2>Proxy Settings</h2>
		
		<div class="field">
			<label for="mc-version">Minecraft Version for Plugins</label>
			<input type="text" id="mc-version" value={localProxyMCVersion} on:input={handleVersionInput} placeholder="e.g., 1.21.11" />
			<span class="hint">MC version used when searching/installing plugins from Modrinth (e.g., 1.21.11, 1.21.1)</span>
		</div>
	</div>

	<div class="save-bar">
		<button class="btn primary" on:click={saveChanges} disabled={saving}>
			{saving ? 'Saving...' : 'Save Changes'}
		</button>
	</div>
</main>

<style>
	.settings-container {
		max-width: 800px;
		margin: 80px auto;
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

	.save-bar {
		position: fixed;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1rem 2rem;
		background-color: var(--bg-secondary);
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: flex-end;
		z-index: 10;
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
</style>
