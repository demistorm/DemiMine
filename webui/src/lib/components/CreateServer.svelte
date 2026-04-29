<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { api } from '$lib/api';
	import { servers, proxies, loadServers, loadProxies } from '$lib/stores/servers';

	export let show = false;

	const dispatch = createEventDispatcher();

	let step = 1;
	let loading = false;
	let error = '';
	let portConflictUsedBy: string | null = null;
	let versions: string[] = [];
	let versionsLoading = false;
	let versionDropdownOpen = false;
	let showSnapshots = false;

	$: {
		if (showSnapshots !== undefined && type && step === 1) {
			loadVersions(showSnapshots);
		}
	}

	$: {
		if (show && $proxies.length === 0) {
			loadProxies();
		}
	}

	let name = '';
	let type = 'paper';
	let version = '';
	let ram = 2048;
	let hostPort: number | null = 25565;
	let proxyId: number | null = null;
	let domain = '';
	let iconFile: File | null = null;
	let iconPreview: string | null = null;
	let iconError = '';
	let minimotdLine1 = '';
	let minimotdLine2 = '';

	const serverTypes = [
		{ value: 'paper', label: 'Paper' },
		{ value: 'purpur', label: 'Purpur' },
		{ value: 'fabric', label: 'Fabric' },
		{ value: 'neoforge', label: 'NeoForge' },
		{ value: 'forge', label: 'Forge' },
		{ value: 'nanolimbo', label: 'NanoLimbo' }
	];

	$: {
		if (type && step === 1) {
			showSnapshots = false;
			loadVersions(false);
		}
	}

	$: {
		if (show && $proxies.length === 0) {
			loadProxies();
		}
	}

	async function loadVersions(includeAll: boolean) {
		versionsLoading = true;
		try {
			const query = includeAll ? '?all=true' : '';
			const response = await api.get<{ versions: ({ version: string; stable?: boolean } | string)[] }>(`/api/versions/${type}${query}`);
			const rawVersions = response.versions || [];
			versions = rawVersions.map(v => ({
				version: typeof v === 'string' ? v : v.version,
				stable: typeof v === 'string' ? true : (v.stable !== false)
			}));
			if (versions.length > 0) {
				const firstStable = versions.find(v => v.stable);
				version = (firstStable || versions[0]).version;
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

	function nextStep2() {
		step = 3;
	}

	function prevStep() {
		if (step === 3) {
			step = 2;
		} else {
			step = 1;
		}
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
			};

			if (proxyId !== null) {
				body.proxy_id = proxyId;
				if (domain) {
					body.domain = domain;
				}
			} else if (hostPort !== null) {
                body.host_port = hostPort;
            }

            if (minimotdLine1) {
                body.minimotd_line1 = minimotdLine1;
            }
            if (minimotdLine2) {
                body.minimotd_line2 = minimotdLine2;
            }

            let endpoint = '/api/servers';
			if (portConflictUsedBy) {
				endpoint += '?force=true';
			}

			const result = await api.post<{ id: number }>(endpoint, body);
			
			if (iconFile && result.id) {
				try {
					await api.uploadIcon(result.id, iconFile);
				} catch (iconErr) {
					console.error('Failed to upload icon:', iconErr);
				}
			}
			
			await loadServers();
			dispatch('created');
			close();
		} catch (err) {
			if (err && typeof err === 'object' && 'error' in err && err.error === 'port_in_use') {
				portConflictUsedBy = (err as any).used_by || 'another server';
				error = '';
			} else {
				error = err instanceof Error ? err.message : 'Failed to create server';
			}
		} finally {
			loading = false;
		}
	}

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

	function clearIcon() {
		if (iconPreview) {
			URL.revokeObjectURL(iconPreview);
		}
		iconFile = null;
		iconPreview = null;
		iconError = '';
	}

	function handlePortChange() {
		portConflictUsedBy = null;
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
        domain = '';
        portConflictUsedBy = null;
        showSnapshots = false;
        clearIcon();
        minimotdLine1 = '';
        minimotdLine2 = '';
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

			{#if portConflictUsedBy}
				<div class="warning-banner">
					Port {hostPort} is already used by '{portConflictUsedBy}'. Only one server on this port can run at a time. Click Create again to confirm.
				</div>
			{/if}

			<div class="steps">
				<div class="step-indicator">
					<div class="step-dot" class:active={step === 1}>1</div>
					<div class="step-line" class:active={step >= 2}></div>
					<div class="step-dot" class:active={step === 2}>2</div>
					{#if proxyId !== null}
						<div class="step-line" class:active={step >= 3}></div>
						<div class="step-dot" class:active={step === 3}>3</div>
					{/if}
				</div>
				<div class="step-labels">
					<span class:active={step === 1}>Basic Info</span>
					<span class:active={step === 2}>Configuration</span>
					{#if proxyId !== null}
						<span class:active={step === 3}>MiniMOTD</span>
					{/if}
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
													version = v.version;
													versionDropdownOpen = false;
												}}
											>
												{v.version}
											</li>
										{/each}
									</ul>
								{/if}
							</div>
						{/if}
						<div class="checkbox-group">
							<label class="checkbox-label">
								<input type="checkbox" bind:checked={showSnapshots} />
								<span>Show snapshot versions</span>
							</label>
						</div>
					</div>

					<div class="actions">
						<button class="btn secondary" on:click={close}>Cancel</button>
						<button class="btn primary" on:click={nextStep} disabled={!name || !version}>
							Next
						</button>
					</div>
				</div>
			{:else if step === 2}
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
									checked={proxyId !== null}
									on:change={() => { 
										proxyId = $proxies.length > 0 ? $proxies[0].id : null; 
										hostPort = null; 
									}}
								/>
								<span>Behind Proxy</span>
							</label>
						</div>
					</div>

					{#if proxyId !== null}
						<div class="field">
							<label for="proxy">Assign to Proxy</label>
							<select id="proxy" bind:value={proxyId}>
								{#each $proxies as proxy}
									<option value={proxy.id}>{proxy.name}</option>
								{/each}
							</select>
							{#if $proxies.length === 0}
								<span class="hint warning">No proxies available. Create a proxy first.</span>
							{/if}
						</div>
						<div class="field">
							<label for="domain">Custom Domain (Optional)</label>
							<input
								type="text"
								id="domain"
								bind:value={domain}
								placeholder="play.example.com"
							/>
							<span class="hint">Players connecting with this domain will be routed to this server</span>
						</div>
					{:else if hostPort !== null}
						<div class="field">
							<label for="port">Host Port</label>
							<input 
								type="number" 
								id="port" 
								bind:value={hostPort}
								on:change={handlePortChange}
								min={1}
								max={65535}
							/>
							<span class="hint">The port players will connect to</span>
						</div>
					{/if}

					<div class="field">
						<label>Server Icon (Optional)</label>
						<div class="icon-section">
							{#if iconPreview}
								<div class="icon-preview-container">
									<img src={iconPreview} alt="Server icon" class="icon-preview" />
									<button class="btn small" on:click={clearIcon}>
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
						</div>
						{#if iconError}
							<span class="hint warning">{iconError}</span>
						{:else}
							<span class="hint">64x64 PNG image for the server list</span>
						{/if}
					</div>

					<div class="actions">
						<button class="btn secondary" on:click={prevStep}>Back</button>
						{#if proxyId !== null}
							<button class="btn primary" on:click={nextStep2} disabled={loading}>
								Next
							</button>
						{:else}
							<button class="btn primary" on:click={createServer} disabled={loading}>
								{loading ? 'Creating...' : 'Create Server'}
							</button>
						{/if}
					</div>
				</div>
			{:else if step === 3}
				<div class="form-step">
					<div class="section">
						<h2>MiniMOTD Configuration</h2>
						<div class="info-box">
							Configure the server list message that players will see for this server.
						</div>
					</div>

					<div class="field">
						<label for="minimotdLine1">Line 1</label>
						<input 
							type="text" 
							id="minimotdLine1" 
							bind:value={minimotdLine1} 
							placeholder="e.g., &lt;blue&gt;Welcome!&lt;/blue&gt;" 
						/>
						<span class="hint">MiniMOTD will apply color codes automatically</span>
					</div>

					<div class="field">
						<label for="minimotdLine2">Line 2</label>
						<input 
							type="text" 
							id="minimotdLine2" 
							bind:value={minimotdLine2} 
							placeholder="e.g., &lt;gradient:blue:red&gt;Custom message&lt;/gradient&gt;" 
						/>
						<span class="hint">MiniMOTD will apply color codes automatically</span>
					</div>

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

	.warning-banner {
		background: var(--warning, #f59e0b);
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
	.field input[type="number"],
	.field select {
		width: 100%;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 3px solid var(--border);
		border-radius: 0;
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

	.hint.warning {
		color: var(--warning, #f59e0b);
		opacity: 1;
	}

	.version-select {
		position: relative;
		width: 100%;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 3px solid var(--border);
		border-radius: 0;
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
		border: 3px solid var(--border);
		border-radius: 0;
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

	.checkbox-group {
		margin-top: 0.5rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text-secondary);
		font-size: 0.8125rem;
		cursor: pointer;
	}

	.checkbox-label input[type="checkbox"] {
		width: auto;
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

	.hint.warning {
		color: var(--warning, #f59e0b);
		opacity: 1;
	}

	.section {
		margin-bottom: 1.5rem;
	}

	.section h2 {
		margin: 0 0 0.75rem 0;
		color: var(--text-primary);
		font-size: 1rem;
	}

	.info-box {
		background: var(--bg-tertiary);
		padding: 0.75rem 1rem;
		border-radius: 0;
		font-size: 0.875rem;
		color: var(--text-secondary);
		border-left: 3px solid var(--accent);
	}
</style>
