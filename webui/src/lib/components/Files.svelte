<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	export let id: number;
	export let type: 'server' | 'proxy' = 'server';

	$: apiPrefix = type === 'server' ? '/api/servers' : '/api/proxies';

	interface FileEntry {
		name: string;
		is_dir: boolean;
		size: number;
		modified: string;
	}

	let currentPath = '/';
	let files: FileEntry[] = [];
	let loading = false;
	let selectedFile: FileEntry | null = null;
	let fileContent = '';
	let editingFile = false;
	let showRenameModal = false;
	let renameOldPath = '';
	let renameNewName = '';
	let isDragging = false;
	let uploadLoading = false;
	let error = '';
	let isGzipped = false;
	let searchTerm = '';

	$: filteredContent = searchTerm 
		? fileContent.split('\n').filter(line => line.toLowerCase().includes(searchTerm.toLowerCase())).join('\n')
		: fileContent;

	onMount(() => {
		loadFiles();
	});

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			if (editingFile) {
				closeEditor();
			} else if (currentPath !== '/') {
				navigateUp();
			}
		}
	}

	async function loadFiles(path = currentPath) {
		loading = true;
		error = '';
		try {
			const response = await api.get<{ path: string; entries: FileEntry[] }>(
				`${apiPrefix}/${id}/files?path=${encodeURIComponent(path)}`
			);
			currentPath = response.path;
			files = response.entries || [];
			files.sort((a, b) => {
				if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
				return a.name.localeCompare(b.name);
			});
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load files';
			console.error('Failed to load files:', err);
		} finally {
			loading = false;
		}
	}

	function navigateTo(path: string) {
		selectedFile = null;
		editingFile = false;
		loadFiles(path);
	}

	function navigateUp() {
		const parts = currentPath.split('/').filter(Boolean);
		parts.pop();
		navigateTo('/' + parts.join('/'));
	}

	function navigateToSegment(index: number) {
		const parts = currentPath.split('/').filter(Boolean);
		const newPath = '/' + parts.slice(0, index + 1).join('/');
		navigateTo(newPath);
	}

	async function openFile(file: FileEntry) {
		if (file.is_dir) {
			const newPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
			navigateTo(newPath);
			return;
		}

		loading = true;
		isGzipped = file.name.endsWith('.gz');
		searchTerm = '';
		try {
			const response = await api.get<{ content: string; is_gzipped?: string }>(
				`${apiPrefix}/${id}/files/content?path=${encodeURIComponent(currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`)}`
			);
			selectedFile = file;
			fileContent = response.content;
			isGzipped = response.is_gzipped === 'true';
			editingFile = true;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to open file';
			console.error('Failed to open file:', err);
		} finally {
			loading = false;
		}
	}

	async function saveFile() {
		if (!selectedFile) return;
		
		if (isGzipped) {
			error = 'Cannot save compressed files';
			return;
		}
		
		const filePath = currentPath === '/' ? `/${selectedFile.name}` : `${currentPath}/${selectedFile.name}`;
		
		loading = true;
		try {
			await api.put(`${apiPrefix}/${id}/files/content?path=${encodeURIComponent(filePath)}`, { content: fileContent });
			editingFile = false;
			selectedFile = null;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save file';
			console.error('Failed to save file:', err);
		} finally {
			loading = false;
		}
	}

	function closeEditor() {
		editingFile = false;
		selectedFile = null;
		fileContent = '';
		isGzipped = false;
		searchTerm = '';
	}

	async function deleteFile(file: FileEntry) {
		if (!confirm(`Delete ${file.name}?`)) return;

		const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		
		try {
			await api.delete(`${apiPrefix}/${id}/files?path=${encodeURIComponent(filePath)}`);
			loadFiles();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to delete';
			console.error('Failed to delete:', err);
		}
	}

	function downloadFile(file: FileEntry) {
		const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		window.open(`${apiPrefix}/${id}/files/download?path=${encodeURIComponent(filePath)}`, '_blank');
	}

	function openRenameModal(file: FileEntry) {
		renameOldPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		renameNewName = file.name;
		showRenameModal = true;
	}

	async function renameFile() {
		if (!renameNewName.trim()) return;

		try {
			await api.post(`${apiPrefix}/${id}/files/rename`, {
				old_path: renameOldPath,
				new_name: renameNewName
			});
			showRenameModal = false;
			loadFiles();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to rename';
			console.error('Failed to rename:', err);
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
		isDragging = true;
	}

	function handleDragLeave(e: DragEvent) {
		e.preventDefault();
		isDragging = false;
	}

	async function handleDrop(e: DragEvent) {
		e.preventDefault();
		isDragging = false;

		const files = e.dataTransfer?.files;
		if (!files || files.length === 0) return;

		await uploadFiles(files);
	}

	async function uploadFiles(fileList: FileList) {
		uploadLoading = true;
		error = '';

		try {
			for (const file of fileList) {
				const formData = new FormData();
				formData.append('file', file);
				
				const token = localStorage.getItem('token');
				const response = await fetch(`${apiPrefix}/${id}/files/upload?path=${encodeURIComponent(currentPath)}`, {
					method: 'POST',
					headers: token ? { 'Authorization': `Bearer ${token}` } : {},
					body: formData
				});

				if (!response.ok) {
					throw new Error('Upload failed');
				}
			}
			loadFiles();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to upload files';
			console.error('Failed to upload:', err);
		} finally {
			uploadLoading = false;
		}
	}

	function handleFilePicker() {
		const input = document.createElement('input');
		input.type = 'file';
		input.multiple = true;
		input.onchange = (e) => {
			const target = e.target as HTMLInputElement;
			if (target.files) {
				uploadFiles(target.files);
			}
		};
		input.click();
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
		return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
	}

	function formatDate(dateStr: string): string {
		const date = new Date(dateStr);
		return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	}

	$: pathSegments = currentPath.split('/').filter(Boolean);
</script>

<svelte:window on:keydown={handleKeydown} />

{#if editingFile}
	<div class="editor-container">
		<div class="editor-header">
			<div class="filename-container">
				<span class="filename">{selectedFile?.name}</span>
				{#if isGzipped}
					<span class="badge badge-warning">Read-only (Compressed)</span>
				{/if}
			</div>
			<div class="editor-actions">
				<button class="btn" on:click={saveFile} disabled={loading || isGzipped}>Save</button>
				<button class="btn secondary" on:click={closeEditor}>Cancel</button>
			</div>
		</div>
		<div class="editor-toolbar">
			<input 
				type="text" 
				bind:value={searchTerm} 
				placeholder="Search in file... (Ctrl+F)" 
				class="search-input"
			/>
			{#if searchTerm && filteredContent !== fileContent}
				<span class="filter-info">
					Showing {filteredContent.split('\n').length} of {fileContent.split('\n').length} lines
				</span>
			{/if}
		</div>
		<textarea bind:value={filteredContent} class="editor" disabled={loading || isGzipped}></textarea>
	</div>
{:else}
	<div 
		class="file-browser"
		class:dragging={isDragging}
		on:dragover={handleDragOver}
		on:dragleave={handleDragLeave}
		on:drop={handleDrop}
	>
		{#if error}
			<div class="error-banner">
				{error}
				<button on:click={() => error = ''}>×</button>
			</div>
		{/if}

		<div class="toolbar">
			<div class="breadcrumb">
				<button class="breadcrumb-btn" on:click={() => navigateTo('/')}>root</button>
				{#each pathSegments as segment, i}
					<span class="separator">/</span>
					<button class="breadcrumb-btn" on:click={() => navigateToSegment(i)}>
						{segment}
					</button>
				{/each}
			</div>
			<div class="actions">
				<button class="btn" on:click={handleFilePicker} disabled={uploadLoading}>
					{uploadLoading ? 'Uploading...' : 'Upload'}
				</button>
				<button class="btn" on:click={navigateUp} disabled={currentPath === '/'}>
					↑ Up
				</button>
			</div>
		</div>

		{#if loading}
			<div class="loading-state">Loading...</div>
		{:else if files.length === 0}
			<div class="empty-state">This folder is empty. Drop files here to upload.</div>
		{:else}
			<div class="file-list">
				<div class="file-header">
					<span class="col-icon"></span>
					<span class="col-name">Name</span>
					<span class="col-size">Size</span>
					<span class="col-modified">Modified</span>
					<span class="col-actions">Actions</span>
				</div>
				{#each files as file}
					<div class="file-item" class:folder={file.is_dir}>
						<span class="col-icon">
							{#if file.is_dir}📁{:else}📄{/if}
						</span>
						<span class="col-name" on:click={() => openFile(file)}>
							{file.name}
						</span>
						<span class="col-size">{file.is_dir ? '-' : formatSize(file.size)}</span>
						<span class="col-modified">{formatDate(file.modified)}</span>
						<span class="col-actions">
							{#if !file.is_dir}
								<button class="action-btn" on:click={() => downloadFile(file)} title="Download">⬇</button>
							{/if}
							<button class="action-btn" on:click={() => openRenameModal(file)} title="Rename">✎</button>
							<button class="action-btn danger" on:click={() => deleteFile(file)} title="Delete">🗑</button>
						</span>
					</div>
				{/each}
			</div>
		{/if}

		{#if isDragging}
			<div class="drop-overlay">
				Drop files here to upload
			</div>
		{/if}
	</div>
{/if}

{#if showRenameModal}
	<div class="modal-overlay" on:click={() => showRenameModal = false}>
		<div class="modal" on:click|stopPropagation>
			<h3>Rename File</h3>
			<input type="text" bind:value={renameNewName} placeholder="New name" />
			<div class="modal-actions">
				<button class="btn" on:click={renameFile}>Rename</button>
				<button class="btn secondary" on:click={() => showRenameModal = false}>Cancel</button>
			</div>
		</div>
	</div>
{/if}

<style>
	.file-browser {
		display: flex;
		flex-direction: column;
		height: 100%;
		background: var(--bg-primary);
		position: relative;
	}

	.error-banner {
		background: var(--error);
		color: white;
		padding: 0.75rem 1rem;
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.error-banner button {
		background: transparent;
		border: none;
		color: white;
		font-size: 1.25rem;
		cursor: pointer;
		padding: 0 0.5rem;
	}

	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.5rem;
		background: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
	}

	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		flex-wrap: wrap;
	}

	.breadcrumb-btn {
		background: transparent;
		color: var(--text-secondary);
		border: none;
		cursor: pointer;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
		border-radius: 0.25rem;
		transition: all 0.2s;
	}

	.breadcrumb-btn:hover {
		color: var(--accent);
		background: var(--bg-tertiary);
	}

	.separator {
		color: var(--text-secondary);
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.btn {
		padding: 0.5rem 1rem;
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
		background-color: var(--accent);
		border-color: var(--accent);
	}

	.btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.btn.secondary {
		background-color: transparent;
	}

	.btn.secondary:hover:not(:disabled) {
		background-color: var(--bg-secondary);
		border-color: var(--border);
	}

	.file-list {
		flex: 1;
		overflow-y: auto;
	}

	.file-header {
		display: grid;
		grid-template-columns: 40px 1fr 100px 140px 120px;
		padding: 0.75rem 1.5rem;
		background: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
	}

	.file-item {
		display: grid;
		grid-template-columns: 40px 1fr 100px 140px 120px;
		padding: 0.75rem 1.5rem;
		border-bottom: 1px solid var(--border);
		align-items: center;
		transition: background-color 0.2s;
	}

	.file-item:hover {
		background-color: var(--bg-secondary);
	}

	.col-icon {
		font-size: 1.125rem;
	}

	.col-name {
		color: var(--text-primary);
		font-weight: 500;
		cursor: pointer;
	}

	.file-item.folder .col-name {
		color: var(--accent);
	}

	.col-name:hover {
		text-decoration: underline;
	}

	.col-size, .col-modified {
		color: var(--text-secondary);
		font-size: 0.875rem;
	}

	.col-actions {
		display: flex;
		gap: 0.25rem;
		justify-content: flex-end;
	}

	.action-btn {
		background: transparent;
		border: none;
		color: var(--text-secondary);
		cursor: pointer;
		padding: 0.25rem 0.5rem;
		border-radius: 0.25rem;
		font-size: 0.875rem;
		transition: all 0.2s;
	}

	.action-btn:hover {
		background: var(--bg-tertiary);
		color: var(--text-primary);
	}

	.action-btn.danger:hover {
		background: var(--error);
		color: white;
	}

	.loading-state, .empty-state {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 200px;
		color: var(--text-secondary);
	}

	.drop-overlay {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(59, 130, 246, 0.1);
		border: 2px dashed var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.25rem;
		color: var(--accent);
		z-index: 10;
	}

	.editor-container {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.editor-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1rem 1.5rem;
		background: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
	}

	.filename-container {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.filename {
		font-weight: 600;
		color: var(--text-primary);
	}

	.badge {
		padding: 0.25rem 0.5rem;
		border-radius: 0.25rem;
		font-size: 0.75rem;
		font-weight: 500;
	}

	.badge-warning {
		background-color: rgba(245, 158, 11, 0.1);
		color: #f59e0b;
		border: 1px solid rgba(245, 158, 11, 0.3);
	}

	.editor-actions {
		display: flex;
		gap: 0.5rem;
	}

	.editor-toolbar {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1.5rem;
		background: var(--bg-tertiary);
		border-bottom: 1px solid var(--border);
	}

	.search-input {
		flex: 1;
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		background-color: var(--bg-primary);
		color: var(--text-primary);
		font-size: 0.875rem;
	}

	.search-input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.filter-info {
		font-size: 0.75rem;
		color: var(--text-secondary);
		white-space: nowrap;
	}

	.editor {
		flex: 1;
		width: 100%;
		padding: 1rem;
		background: #0a0a0a;
		color: var(--text-primary);
		border: none;
		font-family: 'Courier New', Courier, monospace;
		font-size: 0.875rem;
		line-height: 1.5;
		resize: none;
	}

	.editor:focus {
		outline: none;
	}

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
		padding: 1.5rem;
		border-radius: 0.5rem;
		min-width: 300px;
	}

	.modal h3 {
		margin: 0 0 1rem;
		color: var(--text-primary);
	}

	.modal input {
		width: 100%;
		padding: 0.625rem 0.875rem;
		background: var(--bg-primary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		color: var(--text-primary);
		margin-bottom: 1rem;
	}

	.modal input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.modal-actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
	}
</style>
