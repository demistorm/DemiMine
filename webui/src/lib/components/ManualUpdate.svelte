<script lang="ts">
	import { createEventDispatcher } from 'svelte';
	import { jarUpdateApi, type Server } from '$lib/api';

	export let show = false;
	export let server: Server;

	const dispatch = createEventDispatcher();

	let jarFile: File | null = null;
	let version = '';
	let build = '';
	let loading = false;
	let error = '';
	let wasShown = false;

	// prefill when the modal opens
	$: if (show && !wasShown) {
		version = server.version || '';
		build = '';
		error = '';
		jarFile = null;
		wasShown = true;
	}
	$: if (!show) wasShown = false;

	$: jarLabel = getJarLabel(server.type);
	$: versionLabel = server.type === 'nanolimbo' ? 'NanoLimbo Version' : 'Minecraft Version';
	$: versionPlaceholder = server.type === 'nanolimbo' ? 'e.g., 1.13.1' : 'e.g., 26.2';
	$: usesBuilds = server.type === 'paper' || server.type === 'purpur';
	$: isModdedInstaller = server.type === 'neoforge' || server.type === 'forge';

	function getJarLabel(type: string): string {
		switch (type) {
			case 'fabric':
				return 'Fabric Server Launcher JAR';
			case 'neoforge':
				return 'NeoForge Installer JAR';
			case 'forge':
				return 'Forge Installer JAR';
			default:
				return 'Server JAR';
		}
	}

	function getJarHint(type: string): string {
		switch (type) {
			case 'fabric':
				return 'Download the "Executable Server (.jar)" from fabricmc.net/use/server for the target Minecraft version';
			case 'neoforge':
			case 'forge':
				return 'The installer will re-run automatically on next server start';
			case 'nanolimbo':
				return 'e.g., a community build — official repo updates are still detected afterwards';
			default:
				return 'Plugin updates and update checks will follow the new version';
		}
	}

	function handleFileSelect(e: Event) {
		const target = e.target as HTMLInputElement;
		const file = target.files?.[0];
		if (!file) return;

		error = '';

		if (!file.name.toLowerCase().endsWith('.jar')) {
			error = 'File must be a .jar';
			jarFile = null;
			return;
		}

		jarFile = file;
	}

	async function submit() {
		if (!jarFile || !version.trim()) return;

		loading = true;
		error = '';

		try {
			const buildNum = usesBuilds && build.trim() !== '' ? parseInt(build, 10) : undefined;
			await jarUpdateApi.manualServerUpdate(server.id, jarFile, version.trim(), buildNum);
			dispatch('updated', { version: version.trim() });
			close();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update JAR';
		} finally {
			loading = false;
		}
	}

	function close() {
		show = false;
		jarFile = null;
		version = server.version || '';
		build = '';
		error = '';
	}
</script>

{#if show}
	<div class="modal-overlay" on:click={close}>
		<div class="modal" on:click|stopPropagation>
			<div class="modal-header">
				<h2>Manual Update</h2>
				<button class="close-btn" on:click={close}>×</button>
			</div>

			{#if error}
				<div class="error-banner">{error}</div>
			{/if}

			<div class="form-step">
				{#if server.status === 'running'}
					<div class="info-box warning">
						Server must be stopped before updating the JAR
					</div>
				{/if}

				<div class="field">
					<label>{jarLabel}</label>
					<label class="file-label" class:has-file={jarFile}>
						<span class="file-name">{jarFile ? jarFile.name : 'Choose file...'}</span>
						<input
							type="file"
							accept=".jar"
							on:change={handleFileSelect}
							disabled={loading || server.status === 'running'}
						/>
					</label>
					<span class="hint">{getJarHint(server.type)}</span>
				</div>

				<div class="field">
					<label for="manual-version">{versionLabel}</label>
					<input
						type="text"
						id="manual-version"
						bind:value={version}
						placeholder={versionPlaceholder}
						disabled={loading || server.status === 'running'}
					/>
					<span class="hint">Sets the version used for update checks and plugin compatibility</span>
				</div>

				{#if usesBuilds}
					<div class="field">
						<label for="manual-build">Build Number</label>
						<input
							type="number"
							id="manual-build"
							bind:value={build}
							placeholder="Optional — leave blank if unknown"
							min="0"
							disabled={loading || server.status === 'running'}
						/>
						<span class="hint">Blank means unknown; the latest official build will be offered as an update</span>
					</div>
				{/if}

				{#if isModdedInstaller}
					<div class="info-box">
						The old install scripts are removed so the uploaded installer re-runs on next start
					</div>
				{/if}

				<div class="actions">
					<button class="btn secondary" on:click={close} disabled={loading}>Cancel</button>
					<button
						class="btn primary"
						on:click={submit}
						disabled={loading || !jarFile || !version.trim() || server.status === 'running'}
					>
						{loading ? 'Updating...' : 'Apply Manual Update'}
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

	.field input[type='text'],
	.field input[type='number'] {
		width: 100%;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 3px solid var(--border);
		border-radius: 0;
		color: var(--text-primary);
		font-size: 0.9375rem;
		box-sizing: border-box;
	}

	.field input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.file-label {
		display: block;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-primary);
		border: 3px dashed var(--border);
		color: var(--text-secondary);
		cursor: pointer;
		font-size: 0.9375rem;
	}

	.file-label:hover,
	.file-label.has-file {
		border-color: var(--accent);
	}

	.file-label.has-file .file-name {
		color: var(--text-primary);
	}

	.file-label input[type='file'] {
		display: none;
	}

	.hint {
		display: block;
		color: var(--text-tertiary);
		font-size: 0.75rem;
		margin-top: 0.375rem;
	}

	.info-box {
		background: var(--bg-tertiary);
		border-left: 3px solid var(--accent);
		padding: 0.75rem 1rem;
		color: var(--text-secondary);
		font-size: 0.8125rem;
		margin-bottom: 1.25rem;
	}

	.info-box.warning {
		border-left-color: var(--warning, #f59e0b);
	}

	.actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.75rem;
		margin-top: 1.5rem;
	}

	.btn {
		padding: 0.625rem 1.25rem;
		border: 3px solid var(--border);
		background: var(--bg-tertiary);
		color: var(--text-primary);
		cursor: pointer;
		font-size: 0.875rem;
		font-weight: 600;
	}

	.btn:hover:not(:disabled) {
		border-color: var(--accent);
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn.primary {
		background: var(--accent);
		border-color: var(--accent);
		color: white;
	}

	.btn.secondary {
		background: transparent;
	}
</style>
