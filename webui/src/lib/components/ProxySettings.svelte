<script lang="ts">
	import { createEventDispatcher, onMount } from 'svelte';
	import { api, type Proxy, jarUpdateApi, type JarUpdateInfo } from '$lib/api';
	import { proxies } from '$lib/stores/servers';

	export let proxy: Proxy;

	let name = proxy.name || '';
	let ramMB = proxy.ram_mb || 512;
	let saving = false;
	let error = '';
	let success = '';
	let deleteStep = 0;
	let iconFile: File | null = null;
	let iconPreview: string | null = proxy.icon_path || null;
	let iconError = '';
	let iconUploading = false;

	let updateInfo: JarUpdateInfo | null = null;
	let checkingUpdate = false;
	let updatingJar = false;

	const dispatch = createEventDispatcher();

	async function checkJarUpdate() {
		checkingUpdate = true;
		try {
			updateInfo = await jarUpdateApi.checkProxy(proxy.id);
		} catch (err) {
			console.error('Failed to check for JAR update:', err);
		} finally {
			checkingUpdate = false;
		}
	}

	async function updateJar() {
		if (!updateInfo || !updateInfo.has_update) {
			return;
		}

		updatingJar = true;
		error = '';

		try {
			await jarUpdateApi.updateProxy(proxy.id);
			success = 'JAR updated successfully';
			updateInfo.has_update = false;

			const updatedProxy = await api.get<Proxy>(`/api/proxies/${proxy.id}`);
			proxies.update(list =>
				list.map(p => p.id === proxy.id ? updatedProxy : p)
			);
			Object.assign(proxy, updatedProxy);

			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update JAR';
		} finally {
			updatingJar = false;
		}
	}

	onMount(() => {
		checkJarUpdate();
	});

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
			await api.delete(`/api/proxies/${proxy.id}`);
			dispatch('deleted');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete proxy';
			deleteStep = 0;
		} finally {
			saving = false;
		}
	}

	function cancelDelete() {
		deleteStep = 0;
	}

	function copyToClipboard(text: string) {
		navigator.clipboard.writeText(text);
		success = 'Copied to clipboard';
		setTimeout(() => success = '', 2000);
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
		
		img.onload = async () => {
			if (img.width !== 64 || img.height !== 64) {
				iconError = 'Icon must be exactly 64x64 pixels';
				URL.revokeObjectURL(url);
				return;
			}
			
			iconFile = file;
			iconPreview = url;
			
			await uploadIcon();
		};
		
		img.onerror = () => {
			iconError = 'Failed to load image';
			URL.revokeObjectURL(url);
		};
		
		img.src = url;
	}

	async function uploadIcon() {
		if (!iconFile) return;
		
		iconUploading = true;
		iconError = '';
		
		try {
			await api.uploadProxyIcon(proxy.id, iconFile);
			proxies.update(list => 
				list.map(p => p.id === proxy.id ? { 
					...p, 
					icon_path: `/api/proxies/${proxy.id}/icon`
				} : p)
			);
			success = 'Icon uploaded successfully';
			setTimeout(() => success = '', 3000);
		} catch (err) {
			iconError = err instanceof Error ? err.message : 'Failed to upload icon';
		} finally {
			iconUploading = false;
		}
	}

	async function deleteIcon() {
		iconUploading = true;
		iconError = '';
		
		try {
			await api.deleteProxyIcon(proxy.id);
			if (iconPreview && !iconPreview.startsWith('/api/')) {
				URL.revokeObjectURL(iconPreview);
			}
			iconPreview = null;
			iconFile = null;
			proxies.update(list => 
				list.map(p => p.id === proxy.id ? { 
					...p, 
					icon_path: null
				} : p)
			);
			success = 'Icon removed successfully';
			setTimeout(() => success = '', 3000);
		} catch (err) {
			iconError = err instanceof Error ? err.message : 'Failed to delete icon';
		} finally {
			iconUploading = false;
		}
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
		<h2>Software Updates</h2>

		{#if checkingUpdate}
			<div class="field">
				<span>Checking for updates...</span>
			</div>
		{:else if updateInfo}
			<div class="field">
				<label>Current Version</label>
				<input type="text" value={`Velocity ${proxy.jar_version || 'unknown'} (build ${proxy.jar_build || 'unknown'})`} disabled />
			</div>

			<div class="field">
				<label>Latest Version</label>
				<input type="text" value={`Velocity ${updateInfo.latest_version} (build ${updateInfo.latest_build})`} disabled />
			</div>

			{#if updateInfo.has_update}
				{#if proxy.status === 'running'}
					<div class="field">
						<div class="info-box warning">
							Proxy must be stopped to update the JAR
						</div>
					</div>
				{:else}
					<div class="field">
						<button class="btn primary" on:click={updateJar} disabled={updatingJar}>
							{updatingJar ? 'Updating...' : `Update to ${updateInfo.latest_version}`}
						</button>
					</div>
				{/if}
			{:else}
				<div class="field">
					<div class="info-box">
						Proxy is up to date!
					</div>
				</div>
			{/if}
		{:else}
			<div class="field">
				<div class="info-box warning">
					Failed to check for updates
				</div>
			</div>
		{/if}
	</div>

	<div class="section">
		<h2>General Settings</h2>
		
		<div class="field">
			<label>Proxy Icon</label>
			<div class="icon-section">
				{#if iconPreview}
					<div class="icon-preview-container">
						<img src={iconPreview} alt="Proxy icon" class="icon-preview" />
						<div class="icon-actions">
							<label class="btn small">
								Replace
								<input 
									type="file" 
									accept="image/png"
									on:change={handleIconSelect}
									disabled={iconUploading}
								/>
							</label>
							<button 
								class="btn small danger" 
								on:click={deleteIcon}
								disabled={iconUploading}
							>
								Remove
							</button>
						</div>
					</div>
				{:else}
					<div class="icon-upload-container">
						<label class="icon-dropzone" class:uploading={iconUploading}>
							<input 
								type="file" 
								accept="image/png"
								on:change={handleIconSelect}
								disabled={iconUploading}
							/>
							{#if iconUploading}
								<span>Uploading...</span>
							{:else}
								<span>Click to upload 64x64 PNG</span>
							{/if}
						</label>
					</div>
				{/if}
			</div>
			{#if iconError}
				<span class="hint warning">{iconError}</span>
			{:else}
				<span class="hint">64x64 PNG image displayed on canvas (cosmetic only)</span>
			{/if}
		</div>
		
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
			{#if deleteStep === 2}
				<div class="confirm-delete">
					<p class="final-warning">FINAL WARNING: All proxy files and connected server configurations will be affected!</p>
					<div class="confirm-buttons">
						<button class="btn danger" on:click={deleteProxy} disabled={saving}>
							Delete Permanently
						</button>
						<button class="btn" on:click={cancelDelete}>
							Cancel
						</button>
					</div>
				</div>
			{:else if deleteStep === 1}
				<div class="confirm-delete">
					<p>Are you sure? This will delete all proxy files and cannot be undone.</p>
					<div class="confirm-buttons">
						<button class="btn danger" on:click={deleteProxy}>
							Yes, Continue
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

	.icon-section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.icon-preview-container {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.icon-preview {
		width: 64px;
		height: 64px;
		border-radius: 0.375rem;
		object-fit: contain;
		background: var(--bg-tertiary);
	}

	.icon-actions {
		display: flex;
		gap: 0.5rem;
	}

	.icon-actions input {
		display: none;
	}

	.icon-actions label {
		display: inline-block;
		cursor: pointer;
		user-select: none;
		box-sizing: border-box;
		line-height: 1.5;
		vertical-align: middle;
		margin: 0;
		padding: 0;
	}

	.icon-upload-container {
		display: flex;
		align-items: center;
	}

	.icon-dropzone {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 80px;
		height: 80px;
		border: 2px dashed var(--border);
		border-radius: 0.375rem;
		cursor: pointer;
		transition: border-color 0.2s, background-color 0.2s;
		font-size: 0.75rem;
		color: var(--text-secondary);
		text-align: center;
		padding: 0.5rem;
	}

	.icon-dropzone:hover:not(.uploading) {
		border-color: var(--accent);
		background-color: var(--bg-tertiary);
	}

	.icon-dropzone.uploading {
		opacity: 0.6;
		cursor: not-allowed;
	}

	.icon-dropzone input {
		display: none;
	}

	.btn.small {
		padding: 0.375rem 0.75rem;
		font-size: 0.8125rem;
	}

	.hint.warning {
		color: var(--error);
		opacity: 1;
	}

	.info-box.warning {
		background: rgba(234, 179, 8, 0.1);
		border-color: rgba(234, 179, 8, 0.3);
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
