<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import { loadProxies } from '$lib/stores/servers';

	export let show = false;

	const dispatch = createEventDispatcher();

	let step = 1;
	let loading = false;
	let error = '';

	let name = '';
	let hostPort = 25565;
	let ramMB = 512;
	let installDemiAuth = false;
	let installDemiDynamic = false;
	let installLuckPerms = false;
	let iconFile: File | null = null;
	let iconPreview: string | null = null;
	let iconError = '';
	let createdProxyId: number | null = null;
	let motdLine1 = '';
	let motdLine2 = '';

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

	function nextStep() {
		if (!name) return;
		step++;
	}

	function prevStep() {
		step--;
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
				install_demidynamic: installDemiDynamic,
				install_luckperms: installLuckPerms,
				motd_line1: motdLine1,
				motd_line2: motdLine2
			}) as { id: number };

			createdProxyId = result.id;

			if (iconFile && createdProxyId) {
				await api.uploadProxyIcon(createdProxyId, iconFile);
			}

			await loadProxies();
			dispatch('created');
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
		installDemiAuth = false;
		installDemiDynamic = false;
		installLuckPerms = false;
		iconFile = null;
		iconPreview = null;
		iconError = '';
		createdProxyId = null;
		motdLine1 = '';
		motdLine2 = '';
		error = '';
		step = 1;
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

			<div class="steps">
				<div class="step-indicator">
					<div class="step-dot" class:active={step === 1}>1</div>
					<div class="step-line" class:active={step >= 2}></div>
					<div class="step-dot" class:active={step === 2}>2</div>
					<div class="step-line" class:active={step >= 3}></div>
					<div class="step-dot" class:active={step === 3}>3</div>
				</div>
				<div class="step-labels">
					<span class:active={step === 1}>Basic Info</span>
					<span class:active={step === 2}>Plugins</span>
					<span class:active={step === 3}>MiniMOTD</span>
				</div>
			</div>

			{#if step === 1}
				<div class="form-step">
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

					<div class="actions">
						<button class="btn secondary" on:click={close}>Cancel</button>
						<button class="btn primary" on:click={nextStep} disabled={!name}>Next</button>
					</div>
				</div>
			{:else if step === 2}
				<div class="form-step">
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
								<input type="checkbox" bind:checked={installDemiDynamic} />
								<span>Install DemiDynamic (auto start/stop servers)</span>
							</label>
							<span class="hint">Requires API key configuration after creation</span>
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
							<span class="hint">64x64 PNG image for canvas and MOTD (cosmetic only for canvas)</span>
						</div>
					</div>

					<div class="info-box">
						<p>Velocity proxies route players to backend servers. Create servers and assign them to this proxy from the server settings.</p>
					</div>

					<div class="actions">
						<button class="btn secondary" on:click={prevStep}>Back</button>
						<button class="btn primary" on:click={nextStep}>Next</button>
					</div>
				</div>
			{:else if step === 3}
				<div class="form-step">
					<div class="section">
						<h2>MiniMOTD Configuration</h2>
						<div class="info-box">
							Configure the default MOTD shown for backend servers that don't have their own MOTD configured.
						</div>
					</div>

					<div class="field">
						<label for="motdLine1">Line 1</label>
						<input
							type="text"
							id="motdLine1"
							bind:value={motdLine1}
							placeholder="e.g., &lt;blue&gt;Welcome!&lt;/blue&gt;"
						/>
						<span class="hint">MiniMOTD will apply color codes automatically</span>
					</div>

					<div class="field">
						<label for="motdLine2">Line 2</label>
						<input
							type="text"
							id="motdLine2"
							bind:value={motdLine2}
							placeholder="e.g., &lt;gradient:blue:red&gt;Custom message&lt;/gradient&gt;"
						/>
						<span class="hint">MiniMOTD will apply color codes automatically</span>
					</div>

					<div class="actions">
						<button class="btn secondary" on:click={prevStep}>Back</button>
						<button class="btn primary" on:click={createProxy} disabled={loading}>
							{loading ? 'Creating...' : 'Create Proxy'}
						</button>
					</div>
				</div>
			{/if}
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
		border: 3px solid var(--border);
		border-radius: 0;
		width: 90%;
		max-width: 500px;
		max-height: 90vh;
		overflow-y: auto;
	}

	.modal-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.25rem 1.5rem;
		border-bottom: 3px solid var(--border);
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

	.steps {
		padding: 1rem 1.5rem;
		border-bottom: 3px solid var(--border);
	}

	.step-indicator {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0;
		margin-bottom: 0.5rem;
	}

	.step-dot {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--bg-tertiary);
		color: var(--text-secondary);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.75rem;
		font-weight: 600;
		transition: all 0.2s;
	}

	.step-dot.active {
		background: var(--accent);
		color: white;
	}

	.step-line {
		width: 60px;
		height: 2px;
		background: var(--bg-tertiary);
		transition: all 0.2s;
	}

	.step-line.active {
		background: var(--accent);
	}

	.step-labels {
		display: flex;
		justify-content: center;
	}

	.step-labels span {
		width: 88px;
		text-align: center;
		color: var(--text-secondary);
		font-size: 0.75rem;
		transition: all 0.2s;
	}

	.step-labels span.active {
		color: var(--text-primary);
		font-weight: 500;
	}

	.form-step {
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
		border: 3px solid var(--border);
		border-radius: 0;
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

	.checkbox-group {
		margin-bottom: 1rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text-primary);
		font-size: 0.875rem;
		cursor: pointer;
	}

	.checkbox-label input[type="checkbox"] {
		width: auto;
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
		border-radius: 0;
		background-color: var(--bg-tertiary);
		padding: 0.25rem;
	}

	.upload-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 1rem;
		background-color: var(--bg-tertiary);
		border: 6px dashed var(--border);
		border-radius: 0;
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
		border-radius: 0;
		padding: 0.75rem 1rem;
		margin-bottom: 1.25rem;
	}

	.info-box p {
		margin: 0;
		color: var(--text-secondary);
		font-size: 0.8125rem;
		line-height: 1.5;
	}

	.section {
		margin-bottom: 1.5rem;
	}

	.section h2 {
		margin: 0 0 0.75rem 0;
		color: var(--text-primary);
		font-size: 1rem;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		padding-top: 1rem;
		border-top: 3px solid var(--border);
	}

	.btn {
		padding: 0.625rem 1.25rem;
		border-radius: 0;
		font-size: 0.875rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		border: 3px solid var(--border);
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
