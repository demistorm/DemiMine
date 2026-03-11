<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import { loadProxies } from '$lib/stores/servers';

	export let show = false;

	const dispatch = createEventDispatcher();

	let loading = false;
	let error = '';

	let name = '';
	let hostPort = 25565;

	async function createProxy() {
		if (!name) return;
		
		loading = true;
		error = '';

		try {
			await api.post('/api/proxies', {
				name,
				host_port: hostPort
			});
			
			await loadProxies();
			dispatch('created');
			show = false;
			close();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create proxy';
		} finally {
			loading = false;
		}
	}

	function close() {
		show = false;
		name = '';
		hostPort = 25565;
		error = '';
	}
</script>

{#if show}
	<div class="modal-overlay" on:click={close}>
		<div class="modal" on:click|stopPropagation>
			<div class="modal-header">
				<h2>Create Proxy</h2>
				<button class="close-btn" on:click={close}>×</button>
			</div>

			{#if error}
				<div class="error-banner">{error}</div>
			{/if}

			<div class="form">
				<div class="field">
					<label for="name">Proxy Name</label>
					<input 
						type="text" 
						id="name" 
						bind:value={name}
						placeholder="My Proxy"
					/>
				</div>

				<div class="field">
					<label for="port">Host Port</label>
					<input 
						type="number" 
						id="port" 
						bind:value={hostPort}
						min={1}
						max={65535}
					/>
					<span class="hint">The port players will connect to (default: 25565)</span>
				</div>

				<div class="info-box">
					<p>Velocity proxies route players to backend servers. Create servers and assign them to this proxy from the server settings.</p>
				</div>

				<div class="actions">
					<button class="btn secondary" on:click={close}>Cancel</button>
					<button class="btn primary" on:click={createProxy} disabled={loading || !name}>
						{loading ? 'Creating...' : 'Create Proxy'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<style>
	.modal-overlay {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.7);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 1000;
	}

	.modal {
		background: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 0.5rem;
		width: 90%;
		max-width: 450px;
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.25rem 1.5rem;
		border-bottom: 1px solid var(--border);
	}

	.modal-header h2 {
		margin: 0;
		color: var(--text-primary);
		font-size: 1.125rem;
	}

	.close-btn {
		background: transparent;
		border: none;
		color: var(--text-secondary);
		font-size: 1.5rem;
		cursor: pointer;
		padding: 0.25rem;
	}

	.close-btn:hover {
		color: var(--text-primary);
	}

	.error-banner {
		background: var(--error);
		color: white;
		padding: 0.75rem 1rem;
	}

	.form {
		padding: 1.5rem;
	}

	.field {
		margin-bottom: 1.25rem;
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
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		font-size: 0.9375rem;
	}

	.field input:focus {
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

	.info-box {
		background: var(--bg-tertiary);
		border-radius: 0.375rem;
		padding: 0.75rem 1rem;
		margin-bottom: 1.25rem;
	}

	.info-box p {
		margin: 0;
		color: var(--text-secondary);
		font-size: 0.8125rem;
		line-height: 1.5;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		padding-top: 1rem;
		border-top: 1px solid var(--border);
	}

	.btn {
		padding: 0.625rem 1.25rem;
		border-radius: 0.375rem;
		font-size: 0.875rem;
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

	.btn.secondary {
		background-color: transparent;
	}

	.btn.secondary:hover:not(:disabled) {
		background-color: var(--bg-tertiary);
		border-color: var(--border);
	}
</style>
