<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import { servers, loadServers } from '$lib/stores/servers';

	export let show = false;

	const dispatch = createEventDispatcher();

	let step = 1;
	let loading = false;
	let error = '';
	let versions: string[] = [];
	let versionsLoading = false;
	let versionDropdownOpen = false;

	let name = '';
	let type = 'paper';
	let version = '';
	let ram = 2048;
	let hostPort: number | null = 25565;
	let proxyId: number | null = null;

	const serverTypes = [
		{ value: 'paper', label: 'Paper' },
		{ value: 'purpur', label: 'Purpur' },
		{ value: 'fabric', label: 'Fabric' },
		{ value: 'neoforge', label: 'NeoForge' },
		{ value: 'forge', label: 'Forge' }
	];

	$: {
		if (type && step === 1) {
			loadVersions();
		}
	}

	async function loadVersions() {
		versionsLoading = true;
		try {
			const response = await api.get<{ versions: ({ version: string } | string)[] }>(`/api/versions/${type}`);
			const rawVersions = response.versions || [];
			versions = rawVersions.map(v => typeof v === 'string' ? v : v.version);
			if (versions.length > 0) {
				version = versions[0];
			}
		} catch (err) {
			console.error('Failed to load versions:', err);
		} finally {
			versionsLoading = false;
		}
	}

	function nextStep() {
		if (!name || !version) return;
		step = 2;
	}

	function prevStep() {
		step = 1;
	}

	async function createServer() {
		if (!name || !version) return;
		
		loading = true;
		error = '';

		try {
			const body: Record<string, any> = {
				name,
				type,
				version,
				ram_mb: ram,
				backup_interval_days: 7,
				auto_shutdown_minutes: 15
			};

			if (proxyId !== null) {
				body.proxy_id = proxyId;
			} else if (hostPort !== null) {
				body.host_port = hostPort;
			}

			await api.post('/api/servers', body);
			
			await loadServers();
			dispatch('created');
			show = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create server';
		} finally {
			loading = false;
		}
	}

	function close() {
		show = false;
		step = 1;
		name = '';
		type = 'paper';
		version = '';
		ram = 2048;
		hostPort = 25565;
		proxyId = null;
		error = '';
	}
</script>

{#if show}
	<div class="modal-overlay" on:click={close}>
		<div class="modal" on:click|stopPropagation>
			<div class="modal-header">
				<h2>Create Server</h2>
				<button class="close-btn" on:click={close}>×</button>
			</div>

			{#if error}
				<div class="error-banner">{error}</div>
			{/if}

			<div class="steps">
				<div class="step-indicator">
					<div class="step-dot" class:active={step === 1}>1</div>
					<div class="step-line" class:active={step === 2}></div>
					<div class="step-dot" class:active={step === 2}>2</div>
				</div>
				<div class="step-labels">
					<span class:active={step === 1}>Basic Info</span>
					<span class:active={step === 2}>Configuration</span>
				</div>
			</div>

			{#if step === 1}
				<div class="form-step">
					<div class="field">
						<label for="name">Server Name</label>
						<input 
							type="text" 
							id="name" 
							bind:value={name}
							placeholder="My Server"
						/>
					</div>

					<div class="field">
						<label for="type">Server Type</label>
						<select id="type" bind:value={type}>
							{#each serverTypes as t}
								<option value={t.value}>{t.label}</option>
							{/each}
						</select>
					</div>

					<div class="field">
						<label for="version">Minecraft Version</label>
						{#if versionsLoading}
							<div class="version-select disabled">
								<span>Loading versions...</span>
							</div>
						{:else if versions.length === 0}
							<div class="version-select disabled">
								<span>No versions available</span>
							</div>
						{:else}
							<div 
								class="version-select"
								class:open={versionDropdownOpen}
								on:click={() => versionDropdownOpen = !versionDropdownOpen}
							>
								<span class="selected-version">{version || 'Select version'}</span>
								<span class="chevron">▼</span>
								{#if versionDropdownOpen}
									<ul class="version-dropdown">
										{#each versions as v}
											<li 
												class="version-option"
												class:selected={v === version}
												on:click|stopPropagation={() => {
													version = v;
													versionDropdownOpen = false;
												}}
											>
												{v}
											</li>
										{/each}
									</ul>
								{/if}
							</div>
						{/if}
					</div>

					<div class="actions">
						<button class="btn secondary" on:click={close}>Cancel</button>
						<button class="btn primary" on:click={nextStep} disabled={!name || !version}>
							Next
						</button>
					</div>
				</div>
			{:else}
				<div class="form-step">
					<div class="field">
						<label for="ram">RAM Allocation (MB)</label>
						<input 
							type="number" 
							id="ram" 
							bind:value={ram}
							min={512}
							max={16384}
							step={512}
						/>
						<span class="hint">Recommended: 2048-4096 MB for most servers</span>
					</div>

					<div class="field">
						<label>Network Configuration</label>
						<div class="radio-group">
							<label class="radio-label">
								<input 
									type="radio" 
									name="network" 
									value="standalone"
									checked={proxyId === null && hostPort !== null}
									on:change={() => { proxyId = null; hostPort = 25565; }}
								/>
								<span>Standalone (direct connection)</span>
							</label>
							<label class="radio-label">
								<input 
									type="radio" 
									name="network" 
									value="proxy"
									checked={proxyId === null && hostPort === null}
									on:change={() => { proxyId = null; hostPort = null; }}
								/>
								<span>Behind Proxy (no direct access)</span>
							</label>
						</div>
					</div>

					{#if hostPort !== null && proxyId === null}
						<div class="field">
							<label for="port">Host Port</label>
							<input 
								type="number" 
								id="port" 
								bind:value={hostPort}
								min={1}
								max={65535}
							/>
							<span class="hint">The port players will connect to</span>
						</div>
					{/if}

					<div class="actions">
						<button class="btn secondary" on:click={prevStep}>Back</button>
						<button class="btn primary" on:click={createServer} disabled={loading}>
							{loading ? 'Creating...' : 'Create Server'}
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
		border: 1px solid var(--border);
		border-radius: 0.5rem;
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

	.steps {
		padding: 1rem 1.5rem;
		border-bottom: 1px solid var(--border);
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
		gap: 60px;
	}

	.step-labels span {
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
	.field input[type="number"],
	.field select {
		width: 100%;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		font-size: 0.9375rem;
	}

	.field input:focus,
	.field select:focus {
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

	.version-select {
		position: relative;
		width: 100%;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		font-size: 0.9375rem;
		cursor: pointer;
		display: flex;
		justify-content: space-between;
		align-items: center;
		transition: border-color 0.2s;
	}

	.version-select:hover {
		border-color: var(--accent);
	}

	.version-select.disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.version-select.open {
		border-color: var(--accent);
	}

	.version-select .chevron {
		font-size: 0.625rem;
		color: var(--text-secondary);
		transition: transform 0.2s;
	}

	.version-select.open .chevron {
		transform: rotate(180deg);
	}

	.version-dropdown {
		position: absolute;
		bottom: calc(100% + 4px);
		left: 0;
		right: 0;
		max-height: 400px;
		overflow-y: auto;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		list-style: none;
		margin: 0;
		padding: 0.25rem 0;
		z-index: 100;
		box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.15);
	}

	.version-option {
		padding: 0.5rem 0.875rem;
		cursor: pointer;
		transition: background-color 0.15s;
		color: var(--text-primary);
	}

	.version-option:hover {
		background-color: var(--bg-secondary);
	}

	.version-option.selected {
		background-color: var(--accent);
		color: white;
	}

	.radio-group {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.radio-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text-primary);
		font-size: 0.875rem;
		cursor: pointer;
	}

	.radio-label input[type="radio"] {
		width: auto;
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
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
