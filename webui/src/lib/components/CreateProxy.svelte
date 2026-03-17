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
	let ramMB = 512;
	let installDemiAuth = false;
	let installLuckPerms = false;
	let iconFile: File | null = null;
	let iconPreview: string | null = null;
	let iconError = '';
	let createdProxyId: number | null = null;

	function handleIconSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;

		iconError = '';
		
		if (!file.type.includes('png')) {
			iconError = 'Icon must be a PNG file';
			return;
		}

		const img = new Image();
		const url = URL.createObjectURL(file);
		
		img.onload = () => {
			if (img.width !== 64 || img.height !== 64) {
				iconError = 'Icon must be exactly 64x64 pixels';
				URL.revokeObjectURL(url);
				return;
			}
			
			iconFile = file;
			iconPreview = url;
		};
		
		img.onerror = () => {
			iconError = 'Failed to load image';
			URL.revokeObjectURL(url);
		};
		
		img.src = url;
	}

	async function createProxy() {
		if (!name) return;
		
		loading = true;
		error = '';

		try {
			const result = await api.post('/api/proxies', {
				name,
				host_port: hostPort,
				ram_mb: ramMB,
				install_demiauth: installDemiAuth,
				install_luckperms: installLuckPerms
			}) as { id: number };
			
			createdProxyId = result.id;
			
			if (iconFile && createdProxyId) {
				await api.uploadProxyIcon(createdProxyId, iconFile);
			}
			
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
		ramMB = 512;
		iconFile = null;
		iconPreview = null;
		iconError = '';
		createdProxyId = null;
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

				<div class="field">
					<label for="ram">Memory (MB)</label>
					<input
						type="number"
						id="ram"
						bind:value={ramMB}
						min={256}
						step={256}
					/>
					<span class="hint">Java heap size (default: 512)</span>
				</div>

				<div class="field">
					<label>Plugins (Optional)</label>
					<div class="checkbox-group">
						<label class="checkbox-label">
							<input type="checkbox" bind:checked={installDemiAuth} />
							<span>Install DemiAuth (chat-based authentication)</span>
						</label>
						<span class="hint">Requires NanoLimbo server named "login" or "auth"</span>
					</div>
					<div class="checkbox-group">
						<label class="checkbox-label">
							<input type="checkbox" bind:checked={installLuckPerms} />
							<span>Install LuckPerms (permissions)</span>
						</label>
					</div>
				</div>

				<div class="field">
					<label>Proxy Icon (Optional)</label>
					<div class="icon-section">
						{#if iconPreview}
							<div class="icon-preview-container">
								<img src={iconPreview} alt="Proxy icon" class="icon-preview" />
								<button class="btn small" on:click={() => { iconFile = null; iconPreview = null; }}>
									Clear
								</button>
							</div>
						{:else}
							<label class="upload-btn">
								<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
									<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
									<polyline points="17 8 12 3 7 8"></polyline>
									<line x1="12" y1="3" x2="12" y2="15"></line>
								</svg>
								<span>Upload Icon</span>
								<input 
									type="file" 
									accept="image/png"
									on:change={handleIconSelect}
								/>
							</label>
						{/if}
						{#if iconError}
							<div class="icon-error">{iconError}</div>
						{/if}
						<span class="hint">64x64 PNG image for canvas display (cosmetic only)</span>
					</div>
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
		max-width: 500px;
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

	.icon-section {
		margin-bottom: 0.5rem;
	}

	.icon-preview-container {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.icon-preview {
		width: 64px;
		height: 64px;
		object-fit: contain;
		border-radius: 0.25rem;
		background-color: var(--bg-tertiary);
		padding: 0.25rem;
	}

	.upload-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		background-color: var(--bg-tertiary);
		border: 2px dashed var(--border);
		border-radius: 0.5rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.upload-btn:hover {
		border-color: var(--accent);
		background-color: var(--bg-secondary);
	}

	.upload-btn input {
		display: none;
	}

	.upload-btn svg {
		color: var(--text-secondary);
	}

	.upload-btn span {
		color: var(--text-secondary);
		font-size: 0.875rem;
	}

	.icon-error {
		color: var(--error);
		font-size: 0.875rem;
		margin-top: 0.5rem;
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

	.btn.small {
		padding: 0.5rem 0.75rem;
		font-size: 0.8125rem;
	}
</style>
