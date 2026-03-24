<script lang="ts">
	import { onMount } from 'svelte';
	import { loadGlobalSettings, globalSettings, saveGlobalSettings, backgroundTextureUrl, serverTileTextureUrl, proxyTileTextureUrl, loadBackgroundTexture, loadServerTileTexture, loadProxyTileTexture, type GlobalSettings } from '$lib/stores/settings';
	import { settingsApi, backupsApi, type Backup } from '$lib/api';
	import { formatBytes, formatDate } from '$lib/utils';

	let loading = true;
	let saving = false;
	let error = '';
	let success = '';

	let backupHour = 3;
	let backupMinute = 0;
	let backupIntervalDays = '3';
	let retentionCount = 2;
	let backups: Backup[] = [];
	let backupInProgress = false;
	let restoreFile: File | null = null;
	let restoreFileName = '';
	let restoring = false;

	let textureFile: File | null = null;
	let textureFileName = '';
	let textureUploading = false;
	let textureRemoving = false;
	let textureDropZone = false;

	let serverTextureFile: File | null = null;
	let serverTextureFileName = '';
	let serverTextureUploading = false;
	let serverTextureRemoving = false;
	let serverTextureDropZone = false;

	let proxyTextureFile: File | null = null;
	let proxyTextureFileName = '';
	let proxyTextureUploading = false;
	let proxyTextureRemoving = false;
	let proxyTextureDropZone = false;

	$: globalSettings;
	$: backgroundTextureUrl, serverTileTextureUrl, proxyTileTextureUrl;

	onMount(async () => {
		await loadGlobalSettings();
		await loadBackgroundTexture();
		await loadServerTileTexture();
		await loadProxyTileTexture();
		const [h, m] = $globalSettings.backup_time.split(':').map(Number);
		backupHour = h;
		backupMinute = m;
		backupIntervalDays = $globalSettings.backup_interval_days;
		retentionCount = $globalSettings.retention_count;
		await loadBackups();
     loading = false;
	 });

	async function loadBackups() {
    try {
      backups = await backupsApi.list();
    } catch (err) {
      console.error('Failed to load backups:', err);
    }
  }

 	async function saveSettings() {
    saving = true;
    error = '';
    success = '';

    try {
      await saveGlobalSettings({
        proxy_mc_version: $globalSettings.proxy_mc_version,
        backup_time: `${String(backupHour).padStart(2, '0')}:${String(backupMinute).padStart(2, '0')}`,
        backup_interval_days: backupIntervalDays,
        retention_count: retentionCount,
        background_texture_scale: $globalSettings.background_texture_scale
      });
      success = 'Settings saved successfully';
      setTimeout(() => success = '', 3000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to save settings';
    } finally {
      saving = false;
    }
  }

  async function triggerBackup() {
    if (backupInProgress) return;
    
    backupInProgress = true;
    try {
      await backupsApi.create();
      await loadBackups();
      success = 'Backup completed successfully';
      setTimeout(() => success = '', 3000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to create backup';
    } finally {
      backupInProgress = false;
    }
  }

  async function downloadBackup(backup: Backup) {
    try {
      const blob = await backupsApi.download(backup.id);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = backup.archive_path.split('/').pop() || `backup-${backup.id}.tar.zst`;
      a.click();
      window.URL.revokeObjectURL(url);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to download backup';
    }
  }

  async function deleteBackup(backup: Backup) {
    if (!confirm('Are you sure you want to delete this backup?')) return;
    
    try {
      await backupsApi.delete(backup.id);
      backups = backups.filter((b) => b.id !== backup.id);
      success = 'Backup deleted';
      setTimeout(() => success = '', 3000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete backup';
    }
  }

  function handleRestoreFile(e: Event) {
    const target = e.target as HTMLInputElement;
    const file = target.files?.[0];
    if (file) {
      restoreFile = file;
      restoreFileName = file.name;
    }
  }

  async function restoreBackup() {
    if (!restoreFile) return;

    if (!confirm('This will replace ALL servers, proxies, and settings. This cannot be undone. Continue?')) {
      return;
    }

    restoring = true;
    try {
      await backupsApi.restore(restoreFile);
      success = 'Restore initiated. The manager will restart.';
      setTimeout(() => window.location.reload(), 2000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to restore backup';
    } finally {
      restoring = false;
    }
  }

  function handleTextureFile(e: Event) {
    const target = e.target as HTMLInputElement;
    const file = target.files?.[0];
    if (file) {
      if (file.type !== 'image/png' && file.type !== 'image/x-png') {
        error = 'Only PNG files are allowed';
        setTimeout(() => error = '', 3000);
        return;
      }
      textureFile = file;
      textureFileName = file.name;
    }
  }

  async function uploadTexture() {
    if (!textureFile) return;

    textureUploading = true;
    error = '';
    success = '';

    try {
      await settingsApi.uploadBackgroundTexture(textureFile);
      await loadBackgroundTexture();
      success = 'Texture uploaded successfully';
      setTimeout(() => success = '', 3000);
      textureFile = null;
      textureFileName = '';
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to upload texture';
    } finally {
      textureUploading = false;
    }
  }

  async function removeTexture() {
    if (!confirm('Are you sure you want to remove the background texture?')) return;

    textureRemoving = true;
    error = '';
    success = '';

    try {
      await settingsApi.deleteBackgroundTexture();
      backgroundTextureUrl.set(null);
      success = 'Texture removed successfully';
      setTimeout(() => success = '', 3000);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to remove texture';
    } finally {
      textureRemoving = false;
    }
  }

  function handleTextureDrop(e: DragEvent) {
    e.preventDefault();
    textureDropZone = false;
    const file = e.dataTransfer?.files[0];
    if (file) {
      if (file.type !== 'image/png' && file.type !== 'image/x-png') {
        error = 'Only PNG files are allowed';
        setTimeout(() => error = '', 3000);
        return;
      }
      textureFile = file;
      textureFileName = file.name;
    }
  }

  function handleTextureDragOver(e: DragEvent) {
    e.preventDefault();
    textureDropZone = true;
  }

	function handleTextureDragLeave(e: DragEvent) {
		e.preventDefault();
		textureDropZone = false;
	}

	function handleServerTextureFile(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (file) {
			if (file.type !== 'image/png' && file.type !== 'image/x-png') {
				error = 'Only PNG files are allowed';
				setTimeout(() => error = '', 3000);
				return;
			}
			serverTextureFile = file;
			serverTextureFileName = file.name;
		}
	}

	async function uploadServerTexture() {
		if (!serverTextureFile) return;

		serverTextureUploading = true;
		error = '';
		success = '';

		try {
			await settingsApi.uploadServerTileTexture(serverTextureFile);
			await loadServerTileTexture();
			success = 'Texture uploaded successfully';
			setTimeout(() => success = '', 3000);
			serverTextureFile = null;
			serverTextureFileName = '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to upload texture';
		} finally {
			serverTextureUploading = false;
		}
	}

	async function removeServerTexture() {
		if (!confirm('Are you sure you want to remove the server tile texture?')) return;

		serverTextureRemoving = true;
		error = '';
		success = '';

		try {
			await settingsApi.deleteServerTileTexture();
			serverTileTextureUrl.set(null);
			success = 'Texture removed successfully';
			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to remove texture';
		} finally {
			serverTextureRemoving = false;
		}
	}

	function handleServerTextureDrop(e: DragEvent) {
		e.preventDefault();
		serverTextureDropZone = false;
		const file = e.dataTransfer?.files[0];
		if (file) {
			if (file.type !== 'image/png' && file.type !== 'image/x-png') {
				error = 'Only PNG files are allowed';
				setTimeout(() => error = '', 3000);
				return;
			}
			serverTextureFile = file;
			serverTextureFileName = file.name;
		}
	}

	function handleServerTextureDragOver(e: DragEvent) {
		e.preventDefault();
		serverTextureDropZone = true;
	}

	function handleServerTextureDragLeave(e: DragEvent) {
		e.preventDefault();
		serverTextureDropZone = false;
	}

	function handleProxyTextureFile(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (file) {
			if (file.type !== 'image/png' && file.type !== 'image/x-png') {
				error = 'Only PNG files are allowed';
				setTimeout(() => error = '', 3000);
				return;
			}
			proxyTextureFile = file;
			proxyTextureFileName = file.name;
		}
	}

	async function uploadProxyTexture() {
		if (!proxyTextureFile) return;

		proxyTextureUploading = true;
		error = '';
		success = '';

		try {
			await settingsApi.uploadProxyTileTexture(proxyTextureFile);
			await loadProxyTileTexture();
			success = 'Texture uploaded successfully';
			setTimeout(() => success = '', 3000);
			proxyTextureFile = null;
			proxyTextureFileName = '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to upload texture';
		} finally {
			proxyTextureUploading = false;
		}
	}

	async function removeProxyTexture() {
		if (!confirm('Are you sure you want to remove the proxy tile texture?')) return;

		proxyTextureRemoving = true;
		error = '';
		success = '';

		try {
			await settingsApi.deleteProxyTileTexture();
			proxyTileTextureUrl.set(null);
			success = 'Texture removed successfully';
			setTimeout(() => success = '', 3000);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to remove texture';
		} finally {
			proxyTextureRemoving = false;
		}
	}

	function handleProxyTextureDrop(e: DragEvent) {
		e.preventDefault();
		proxyTextureDropZone = false;
		const file = e.dataTransfer?.files[0];
		if (file) {
			if (file.type !== 'image/png' && file.type !== 'image/x-png') {
				error = 'Only PNG files are allowed';
				setTimeout(() => error = '', 3000);
				return;
			}
			proxyTextureFile = file;
			proxyTextureFileName = file.name;
		}
	}

	function handleProxyTextureDragOver(e: DragEvent) {
		e.preventDefault();
		proxyTextureDropZone = true;
	}

	function handleProxyTextureDragLeave(e: DragEvent) {
		e.preventDefault();
		proxyTextureDropZone = false;
	}
</script>

<svelte:head>
  <title>Settings - DemiMine</title>
</svelte:head>

 <main class="settings-wrapper">
  <div class="settings-container">
  {#if error}
    <div class="alert error">{error}<button on:click={() => error = ''}>×</button></div>
  {/if}
  {#if success}
    <div class="alert success">{success}</div>
  {/if}

  {#if loading}
    <div class="loading">Loading...</div>
  {:else}
    <div class="section api-keys-section">
      <h2>API Keys</h2>
      <p class="hint">Generate and manage API keys for external integrations like the DemiDynamic plugin.</p>
      <div class="button-wrapper">
        <a href="/api-keys" class="btn primary">Manage API Keys</a>
      </div>
    </div>

    <div class="section">
      <h2>Appearance</h2>
      <p class="hint">Customize textures for the canvas background and tiles with 16x16 pixel art PNGs.</p>

      <div class="field">
        <label>Texture Scale Factor</label>
        <input type="range" min="1" max="12" step="1" bind:value={$globalSettings.background_texture_scale} />
        <span class="hint">Scale factor: {$globalSettings.background_texture_scale}x (applies to all textures, 1x = 16px, 4x = 64px)</span>
      </div>

      <h3>Background Texture</h3>
      <div class="texture-preview">
        {#if $backgroundTextureUrl}
          <div class="texture-preview-inner">
            <img src={$backgroundTextureUrl} alt="Background texture" />
            <button class="btn small danger" on:click={removeTexture} disabled={textureRemoving}>
              {textureRemoving ? 'Removing...' : 'Remove'}
            </button>
          </div>
        {:else}
          <div class="drop-zone {textureDropZone ? 'drag-over' : ''}"
               on:drop={handleTextureDrop}
               on:dragover={handleTextureDragOver}
               on:dragleave={handleTextureDragLeave}>
            <div class="drop-zone-content">
              <p>Drop a 16x16 PNG texture here, or click to browse</p>
              <input type="file" id="textureFile" accept="image/png" on:change={handleTextureFile} />
              <label for="textureFile" class="browse-btn">Browse</label>
            </div>
          </div>
        {/if}
      </div>

      <div class="field" style="margin-top: 1rem;">
        {#if textureFileName}
          <span class="selected-file">Selected: {textureFileName}</span>
        {/if}
        {#if textureFile && !$backgroundTextureUrl}
          <button class="btn" on:click={uploadTexture} disabled={textureUploading}>
            {textureUploading ? 'Uploading...' : 'Upload Texture'}
          </button>
        {/if}
      </div>

      <h3>Server Tile Texture</h3>
      <div class="texture-preview">
        {#if $serverTileTextureUrl}
          <div class="texture-preview-inner">
            <img src={$serverTileTextureUrl} alt="Server tile texture" />
            <button class="btn small danger" on:click={removeServerTexture} disabled={serverTextureRemoving}>
              {serverTextureRemoving ? 'Removing...' : 'Remove'}
            </button>
          </div>
        {:else}
          <div class="drop-zone {serverTextureDropZone ? 'drag-over' : ''}"
               on:drop={handleServerTextureDrop}
               on:dragover={handleServerTextureDragOver}
               on:dragleave={handleServerTextureDragLeave}>
            <div class="drop-zone-content">
              <p>Drop a 16x16 PNG texture here, or click to browse</p>
              <input type="file" id="serverTextureFile" accept="image/png" on:change={handleServerTextureFile} />
              <label for="serverTextureFile" class="browse-btn">Browse</label>
            </div>
          </div>
        {/if}
      </div>

      <div class="field" style="margin-top: 1rem;">
        {#if serverTextureFileName}
          <span class="selected-file">Selected: {serverTextureFileName}</span>
        {/if}
        {#if serverTextureFile && !$serverTileTextureUrl}
          <button class="btn" on:click={uploadServerTexture} disabled={serverTextureUploading}>
            {serverTextureUploading ? 'Uploading...' : 'Upload Texture'}
          </button>
        {/if}
      </div>

      <h3>Proxy Tile Texture</h3>
      <div class="texture-preview">
        {#if $proxyTileTextureUrl}
          <div class="texture-preview-inner">
            <img src={$proxyTileTextureUrl} alt="Proxy tile texture" />
            <button class="btn small danger" on:click={removeProxyTexture} disabled={proxyTextureRemoving}>
              {proxyTextureRemoving ? 'Removing...' : 'Remove'}
            </button>
          </div>
        {:else}
          <div class="drop-zone {proxyTextureDropZone ? 'drag-over' : ''}"
               on:drop={handleProxyTextureDrop}
               on:dragover={handleProxyTextureDragOver}
               on:dragleave={handleProxyTextureDragLeave}>
            <div class="drop-zone-content">
              <p>Drop a 16x16 PNG texture here, or click to browse</p>
              <input type="file" id="proxyTextureFile" accept="image/png" on:change={handleProxyTextureFile} />
              <label for="proxyTextureFile" class="browse-btn">Browse</label>
            </div>
          </div>
        {/if}
      </div>

      <div class="field" style="margin-top: 1rem;">
        {#if proxyTextureFileName}
          <span class="selected-file">Selected: {proxyTextureFileName}</span>
        {/if}
        {#if proxyTextureFile && !$proxyTileTextureUrl}
          <button class="btn" on:click={uploadProxyTexture} disabled={proxyTextureUploading}>
            {proxyTextureUploading ? 'Uploading...' : 'Upload Texture'}
          </button>
        {/if}
      </div>
    </div>

    <div class="section">
      <h2>Backups</h2>

        <div class="field">
         <label>Backup Time (24h format)</label>
          <div class="time-input">
            <select id="backupHour" bind:value={backupHour} class="time-field">
              {#each Array.from({length: 24}, (_, i) => i) as h}
                <option value={h}>{String(h).padStart(2, '0')}</option>
              {/each}
            </select>
            <span class="time-separator">:</span>
            <select id="backupMinute" bind:value={backupMinute} class="time-field">
              {#each Array.from({length: 60}, (_, i) => i) as m}
                <option value={m}>{String(m).padStart(2, '0')}</option>
              {/each}
            </select>
          </div>
         <span class="hint">Time of day to run scheduled backup (00:00 - 23:59)</span>
       </div>

      <div class="field">
        <label for="backupIntervalDays">Backup Interval (days)</label>
        <input type="number" id="backupIntervalDays" bind:value={backupIntervalDays} min="1" />
        <span class="hint">Run backup every {backupIntervalDays} days (only if manager is running at scheduled time)</span>
      </div>

      <div class="field">
        <label for="retentionCount">Keep Backups</label>
        <input type="number" id="retentionCount" bind:value={retentionCount} min="1" />
        <span class="hint">Number of recent backups to retain (older ones are automatically deleted)</span>
      </div>

      <div class="field">
        <button class="btn" on:click={triggerBackup} disabled={backupInProgress}>
          {backupInProgress ? 'Backing up...' : 'Backup Now'}
        </button>
      </div>

      <h3>Recent Backups</h3>
      {#if backups.length > 0}
        <div class="backup-list">
          {#each backups as backup}
            <div class="backup-item">
              <div class="backup-info">
                <span class="backup-name">{backup.archive_path.split('/').pop()}</span>
                <span class="backup-meta">{formatBytes(backup.size_bytes)} • {formatDate(backup.created_at, $globalSettings.server_timezone)}</span>
              </div>
              <div class="backup-actions">
                <button class="btn small" on:click={() => downloadBackup(backup)}>Download</button>
                <button class="btn small danger" on:click={() => deleteBackup(backup)}>Delete</button>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <p class="no-backups">No backups yet</p>
      {/if}
    </div>

    <div class="section danger">
      <h2>Restore from Backup</h2>
      
      <div class="field">
        <label for="restoreFile">Backup File</label>
        <div class="file-input">
          <input type="file" id="restoreFile" accept=".tar.zst" on:change={handleRestoreFile} />
          <span class="file-name">{restoreFileName || 'Choose a file...'}</span>
        </div>
      </div>

      <div class="warning-box">
        <strong>Warning:</strong> This will replace all servers, proxies, and settings. This action cannot be undone.
      </div>

      <div class="field">
        <button class="btn danger" on:click={restoreBackup} disabled={!restoreFile || restoring}>
          {restoring ? 'Restoring...' : 'Restore Backup'}
        </button>
      </div>
    </div>

    <div class="save-bar">
      <button class="btn primary" on:click={saveSettings} disabled={saving}>
        {saving ? 'Saving...' : 'Save Settings'}
      </button>
    </div>
  {/if}
  </div>
</main>

<style>
  .settings-wrapper {
    height: calc(100vh - 56px);
    margin-top: 80px;
    overflow-y: auto;
  }

  .settings-container {
    max-width: 800px;
    margin: 0 auto;
    padding-bottom: 100px;
  }

  .loading {
    text-align: center;
    padding: 2rem;
    color: var(--text-secondary);
  }

  .alert {
    padding: 0.75rem 1rem;
    border-radius: 0;
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
    border: 3px solid var(--border);
    border-radius: 0;
  }

  .section.danger {
    border-color: var(--error);
    background: rgba(239, 68, 68, 0.05);
  }

  .section.api-keys-section {
    min-height: 150px;
  }

  .button-wrapper {
    margin-top: 1.5rem;
    margin-bottom: 1.5rem;
  }

  .section h2 {
    color: var(--text-primary);
    font-size: 1.125rem;
    margin: 0 0 1.5rem;
    padding-bottom: 0.75rem;
    border-bottom: 3px solid var(--border);
  }

  .section h3 {
    color: var(--text-primary);
    font-size: 0.9375rem;
    margin: 1.5rem 0 1rem;
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
  .field input[type="time"],
  .field input[type="number"],
  .field select {
    width: 100%;
    max-width: 400px;
    padding: 0.625rem 0.875rem;
    background-color: var(--bg-primary);
    border: 3px solid var(--border);
    border-radius: 0;
    color: var(--text-primary);
    font-size: 0.9375rem;
  }

  .field input:focus:not(:disabled),
  .field select:focus:not(:disabled) {
    outline: none;
    border-color: var(--accent);
  }

  .time-input {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .time-field {
    width: 80px !important;
    text-align: left;
  }

  .time-separator {
    font-size: 1.25rem;
    font-weight: bold;
    color: var(--text-primary);
  }

  .hint {
    display: block;
    color: var(--text-secondary);
    font-size: 0.75rem;
    margin-top: 0.375rem;
    opacity: 0.7;
  }

  .backup-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .backup-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem;
    background: var(--bg-tertiary);
    border-radius: 0;
  }

  .backup-info {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .backup-name {
    color: var(--text-primary);
    font-size: 0.875rem;
  }

  .backup-meta {
    color: var(--text-secondary);
    font-size: 0.75rem;
  }

  .backup-actions {
    display: flex;
    gap: 0.5rem;
  }

  .no-backups {
    color: var(--text-secondary);
    font-size: 0.875rem;
    margin: 0;
  }

  .file-input {
    display: flex;
    align-items: center;
    gap: 1rem;
    max-width: 400px;
  }

  .file-input input[type="file"] {
    flex-shrink: 0;
  }

  .file-name {
    color: var(--text-secondary);
    font-size: 0.875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .warning-box {
    padding: 0.75rem;
    background: rgba(234, 179, 8, 0.1);
    border: 3px solid rgba(234, 179, 8, 0.3);
    border-radius: 0;
    font-size: 0.875rem;
    margin-bottom: 1.5rem;
    color: var(--text-secondary);
  }

  .warning-box strong {
    color: var(--text-primary);
  }

  .save-bar {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    padding: 1rem 2rem;
    background-color: var(--bg-secondary);
    border-top: 3px solid var(--border);
    display: flex;
    justify-content: flex-end;
    z-index: 10;
  }

  .btn {
    padding: 0.625rem 1.25rem;
    border-radius: 0;
    font-size: 0.9375rem;
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

  .btn.danger {
    color: var(--error);
    border-color: var(--error);
    background-color: transparent;
  }

  .btn.danger:hover:not(:disabled) {
    background-color: var(--error);
    color: white;
  }

  .btn.small {
    padding: 0.375rem 0.75rem;
    font-size: 0.8125rem;
  }

  .texture-preview {
    margin-top: 1rem;
  }

  .texture-preview-inner {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem;
    background: var(--bg-tertiary);
    border-radius: 0;
  }

  .texture-preview-inner img {
    width: 64px;
    height: 64px;
    image-rendering: pixelated;
    border: 6px solid var(--border);
    border-radius: 0;
    background: var(--bg-primary);
  }

  .drop-zone {
    border: 6px dashed var(--border);
    border-radius: 0;
    padding: 2rem;
    text-align: center;
    transition: all 0.2s;
    background: var(--bg-primary);
  }

  .drop-zone.drag-over {
    border-color: var(--accent);
    background: rgba(59, 130, 246, 0.1);
  }

  .drop-zone-content p {
    color: var(--text-secondary);
    margin-bottom: 1rem;
  }

  .drop-zone-content input[type="file"] {
    display: none;
  }

  .browse-btn {
    display: inline-block;
    padding: 0.5rem 1rem;
    background: var(--bg-tertiary);
    border: 3px solid var(--border);
    border-radius: 0;
    color: var(--text-primary);
    cursor: pointer;
    font-size: 0.875rem;
    transition: all 0.2s;
  }

  .browse-btn:hover {
    background: var(--bg-secondary);
  }

  .selected-file {
    display: inline-block;
    color: var(--text-secondary);
    font-size: 0.875rem;
    margin-right: 1rem;
  }

  .field input[type="range"] {
    width: 100%;
    max-width: 400px;
    padding: 0;
  }
</style>
