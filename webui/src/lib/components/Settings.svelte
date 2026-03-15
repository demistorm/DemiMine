<script lang="ts">
	import { createEventDispatcher, onMount } from 'svelte';
	import { api, type Server, jarUpdateApi, type JarUpdateInfo } from '$lib/api';
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
	let iconFile: File | null = null;
	let iconPreview: string | null = server.icon_path || null;
	let iconError = '';
	let iconUploading = false;
	let minimotdLine1 = server.minimotd_line1 || '';
	let minimotdLine2 = server.minimotd_line2 || '';

	let updateInfo: JarUpdateInfo | null = null;
	let checkingUpdate = false;
	let updatingJar = false;

	const dispatch = createEventDispatcher();

	async function checkJarUpdate() {
		if (server.type !== 'paper' && server.type !== 'purpur') {
			return;
		}

		checkingUpdate = true;
		try {
			updateInfo = await jarUpdateApi.checkServer(server.id);
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
			await jarUpdateApi.updateServer(server.id);
			success = 'JAR updated successfully';
			updateInfo.has_update = false;

			const updatedServer = await api.get<Server>(`/api/servers/${server.id}`);
			servers.update(list =>
				list.map(s => s.id === server.id ? updatedServer : s)
			);
			Object.assign(server, updatedServer);

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
			const body: Record<string, any> = {
				name: name !== server.name ? name : undefined,
				ram_mb: ram !== server.ram_mb ? ram : undefined,
				auto_shutdown_minutes: autoShutdown !== server.auto_shutdown_minutes ? autoShutdown : undefined,
				backup_interval_days: backupInterval !== server.backup_interval_days ? backupInterval : undefined
			};

			if (minimotdLine1 !== (server.minimotd_line1 || '')) {
				body.minimotd_line1 = minimotdLine1;
			}
			if (minimotdLine2 !== (server.minimotd_line2 || '')) {
				body.minimotd_line2 = minimotdLine2;
			}

			await api.patch(`/api/servers/${server.id}`, body);
			
			success = 'Settings saved successfully';
			servers.update(list => 
				list.map(s => s.id === server.id ? { 
					...s, 
					name,
					ram_mb: ram,
					auto_shutdown_minutes: autoShutdown,
					backup_interval_days: backupInterval,
					minimotd_line1: minimotdLine1,
					minimotd_line2: minimotdLine2
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
			await api.uploadIcon(server.id, iconFile);
			servers.update(list => 
				list.map(s => s.id === server.id ? { 
					...s, 
					icon_path: `/api/servers/${server.id}/icon`
				} : s)
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
			await api.deleteIcon(server.id);
			if (iconPreview && !iconPreview.startsWith('/api/')) {
				URL.revokeObjectURL(iconPreview);
			}
			iconPreview = null;
			iconFile = null;
			servers.update(list => 
				list.map(s => s.id === server.id ? { 
					...s, 
					icon_path: null
				} : s)
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

	{#if server.type === 'paper' || server.type === 'purpur'}
		<div class="section">
			<h2>Software Updates</h2>

			{#if checkingUpdate}
				<div class="field">
					<span>Checking for updates...</span>
				</div>
			{:else if updateInfo}
				<div class="field">
					<label>Current Version</label>
					<input type="text" value={`${server.type.charAt(0).toUpperCase() + server.type.slice(1)} ${server.version} (build ${server.jar_build > 0 ? server.jar_build : 'unknown'})`} disabled />
				</div>

				<div class="field">
					<label>Latest Version</label>
					<input type="text" value={`${server.type.charAt(0).toUpperCase() + server.type.slice(1)} ${updateInfo.latest_version} (build ${updateInfo.latest_build})`} disabled />
				</div>

				{#if updateInfo.has_update}
					{#if server.status === 'running'}
						<div class="field">
							<div class="info-box warning">
								Server must be stopped to update the JAR
							</div>
						</div>
					{:else}
						<div class="field">
							<button class="btn primary" on:click={updateJar} disabled={updatingJar}>
								{updatingJar ? 'Updating...' : `Update to Build ${updateInfo.latest_build}`}
							</button>
						</div>
					{/if}
				{:else}
					<div class="field">
						<div class="info-box">
							Server is up to date!
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
	{/if}

	<div class="section">
		<h2>General Settings</h2>
		
		<div class="field">
			<label>Server Icon</label>
			<div class="icon-section">
				{#if iconPreview}
					<div class="icon-preview-container">
						<img src={iconPreview} alt="Server icon" class="icon-preview" />
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
				<span class="hint">64x64 PNG image displayed in the server list</span>
			{/if}
		</div>
		
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

	{#if server.proxy_id}
		<div class="section">
			<h2>MiniMOTD Configuration</h2>

			<div class="field">
				<label for="minimotdLine1">Line 1</label>
				<input type="text" id="minimotdLine1" bind:value={minimotdLine1} placeholder="e.g., &lt;blue&gt;Welcome!&lt;/blue&gt;" />
				<span class="hint">MiniMOTD will apply color codes automatically</span>
			</div>

			<div class="field">
				<label for="minimotdLine2">Line 2</label>
				<input type="text" id="minimotdLine2" bind:value={minimotdLine2} placeholder="e.g., &lt;gradient:blue:red&gt;Custom message&lt;/gradient&gt;" />
				<span class="hint">MiniMOTD will apply color codes automatically</span>
			</div>

			<div class="field">
				<div class="info-box">
					<strong>Note:</strong> Changes to these lines will update the MiniMOTD configuration for this server in the proxy. If you leave both fields blank, the MiniMOTD configuration will be deleted.
				</div>
			</div>
		</div>
	{/if}

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

	.info-box {
		padding: 0.75rem;
		background: rgba(59, 130, 246, 0.1);
		border: 1px solid rgba(59, 130, 246, 0.3);
		border-radius: 0.375rem;
		font-size: 0.875rem;
		line-height: 1.4;
	}

	.info-box strong {
		color: var(--text-primary);
	}

	.info-box.warning {
		background: rgba(234, 179, 8, 0.1);
		border-color: rgba(234, 179, 8, 0.3);
	}

</style>
