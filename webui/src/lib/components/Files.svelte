<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, ApiError } from '$lib/api';
	import { getAceMode } from '$lib/utils/ace-utils';
	import { formatDate } from '$lib/utils';
	import { globalSettings } from '$lib/stores/settings';
	import { inputMode } from '$lib/stores/inputMode';
	import AceEditor from '$lib/components/AceEditor.svelte';

	export let id: number;
	export let type: 'server' | 'proxy' = 'server';

	$: apiPrefix = type === 'server' ? '/api/servers' : '/api/proxies';

	interface FileEntry {
		name: string;
		is_dir: boolean;
		size: number;
		modified: string;
	}

	interface LinkInfo {
		id: number;
		source_type: string;
		source_id: number;
		source_name: string;
		source_path: string;
		target_type: string;
		target_id: number;
		target_name: string;
		target_path: string;
		source_missing: boolean;
		source_is_dir: boolean;
	}

	interface LinkCode {
		t: 'server' | 'proxy';
		id: number;
		p: string;
		n: string;
	}

	function isImageFile(filename: string): boolean {
		const ext = filename.split('.').pop()?.toLowerCase();
		return ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico'].includes(ext || '');
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
	let dragCounter = 0;
	let uploadLoading = false;
	let error = '';
	let isGzipped = false;
	let isImageFileOpen = false;
	let imageUrl = '';

	// file links
	let links: LinkInfo[] = [];
	let linkToast = '';
	let showPasteModal = false;
	let pendingPaste: LinkCode | null = null;
	let pasteLoading = false;

	onMount(() => {
		loadFiles();
		loadLinks();
		window.addEventListener('dragend', resetDrag);
		window.addEventListener('dragleave', handleWindowDragLeave);
	});

	onDestroy(() => {
		window.removeEventListener('dragend', resetDrag);
		window.removeEventListener('dragleave', handleWindowDragLeave);
	});

	function resetDrag() {
		dragCounter = 0;
		isDragging = false;
	}

	function handleWindowDragLeave(e: DragEvent) {
		if (e.relatedTarget === null && e.clientX === 0 && e.clientY === 0) {
			resetDrag();
		}
	}

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
		const link = linkFor(file);
		if (file.is_dir || link?.source_is_dir) {
			const newPath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
			navigateTo(newPath);
			return;
		}

		loading = true;
		isGzipped = file.name.endsWith('.gz');
		isImageFileOpen = isImageFile(file.name);
		imageUrl = '';

		if (isImageFileOpen) {
			const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
			imageUrl = `${apiPrefix}/${id}/files/download?path=${encodeURIComponent(filePath)}`;
			selectedFile = file;
			editingFile = true;
			loading = false;
		} else {
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
		isImageFileOpen = false;
		imageUrl = '';
	}

	async function deleteFile(file: FileEntry, force = false) {
		const link = linkFor(file);
		if (link) {
			await removeLink(link);
			return;
		}

		if (!force && !confirm(`Delete ${file.name}?`)) return;

		const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;

		try {
			await api.delete(
				`${apiPrefix}/${id}/files?path=${encodeURIComponent(filePath)}${force ? '&force=true' : ''}`
			);
			await Promise.all([loadFiles(), loadLinks()]);
		} catch (err) {
			if (err instanceof ApiError && err.dependents && err.dependents.length > 0) {
				const names = err.dependents.map((d) => `${d.target_name}:${d.target_path}`).join(', ');
				if (force || confirm(`Other servers link from this (${names}). Delete anyway? This removes those links.`)) {
					await deleteFile(file, true);
				}
				return;
			}
			error = err instanceof Error ? err.message : 'Failed to delete';
			console.error('Failed to delete:', err);
		}
	}

	function downloadFile(file: FileEntry) {
		const filePath = currentPath === '/' ? `/${file.name}` : `${currentPath}/${file.name}`;
		window.open(`${apiPrefix}/${id}/files/download?path=${encodeURIComponent(filePath)}`, '_blank');
	}

	function openRenameModal(file: FileEntry) {
		if (linkFor(file)) {
			error = 'Linked items can\'t be renamed — remove the link first';
			return;
		}
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

	// ---- file links ----

	function relPathFor(name: string): string {
		const p = currentPath === '/' ? `/${name}` : `${currentPath}/${name}`;
		return p.replace(/^\//, '');
	}

	async function loadLinks() {
		try {
			const response = await api.get<{ links: LinkInfo[] }>(
				`/api/links?target_type=${type}&target_id=${id}`
			);
			links = response.links || [];
		} catch (err) {
			console.error('Failed to load links:', err);
		}
	}

	function linkFor(file: FileEntry): LinkInfo | undefined {
		return links.find((l) => l.target_path === relPathFor(file.name));
	}

	function encodeLinkCode(code: LinkCode): string {
		const json = JSON.stringify(code);
		return 'demimine-link:v1:' + btoa(unescape(encodeURIComponent(json)));
	}

	function decodeLinkCode(text: string): LinkCode | null {
		if (!text.startsWith('demimine-link:v1:')) return null;
		try {
			const json = decodeURIComponent(escape(atob(text.slice('demimine-link:v1:'.length))));
			const parsed = JSON.parse(json);
			if (
				parsed &&
				(parsed.t === 'server' || parsed.t === 'proxy') &&
				typeof parsed.id === 'number' &&
				typeof parsed.p === 'string'
			) {
				return parsed as LinkCode;
			}
			return null;
		} catch {
			return null;
		}
	}

	async function copyLink(file: FileEntry) {
		try {
			const entity = await api.get<{ name: string }>(`${apiPrefix}/${id}`);
			const code = encodeLinkCode({ t: type, id, p: relPathFor(file.name), n: entity.name });
			await navigator.clipboard.writeText(code);
			linkToast = 'Link copied — paste it in another server/proxy Files tab';
			setTimeout(() => (linkToast = ''), 3000);
		} catch (err) {
			error = 'Could not copy link to clipboard';
			console.error('Failed to copy link:', err);
		}
	}

	function handlePasteEvent(e: ClipboardEvent) {
		if (editingFile) return;
		const target = e.target as HTMLElement | null;
		if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
			return;
		}
		const text = e.clipboardData?.getData('text') || '';
		const code = decodeLinkCode(text);
		if (code) {
			e.preventDefault();
			pendingPaste = code;
			showPasteModal = true;
		}
	}

	async function pasteFromClipboardButton() {
		try {
			const text = await navigator.clipboard.readText();
			const code = decodeLinkCode(text);
			if (code) {
				pendingPaste = code;
				showPasteModal = true;
			} else {
				error = 'No file link found in clipboard';
			}
		} catch (err) {
			error = 'Could not read clipboard — use Ctrl+V instead';
			console.error('Failed to read clipboard:', err);
		}
	}

	$: pendingPasteTargetPath =
		pendingPaste && currentPath === '/'
			? pendingPaste.p.split('/').pop()
			: pendingPaste
				? `${currentPath}/${pendingPaste.p.split('/').pop()}`
				: '';

	async function confirmPaste() {
		if (!pendingPaste) return;
		pasteLoading = true;
		error = '';
		try {
			await api.post('/api/links', {
				source_type: pendingPaste.t,
				source_id: pendingPaste.id,
				source_path: pendingPaste.p,
				target_type: type,
				target_id: id,
				target_path: pendingPasteTargetPath
			});
			showPasteModal = false;
			pendingPaste = null;
			await Promise.all([loadFiles(), loadLinks()]);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to create link';
			console.error('Failed to create link:', err);
		} finally {
			pasteLoading = false;
		}
	}

	async function removeLink(link: LinkInfo) {
		if (!confirm(`Remove the link '${link.target_path.split('/').pop()}'? The original in ${link.source_name} is not affected.`)) return;
		try {
			await api.delete(`/api/links/${link.id}`);
			await Promise.all([loadFiles(), loadLinks()]);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to remove link';
			console.error('Failed to remove link:', err);
		}
	}

	async function unlinkLocalCopy(link: LinkInfo) {
		if (
			!confirm(
				`Unlink '${link.target_path.split('/').pop()}'? A local copy of the current data will be kept here, and future changes won't sync with ${link.source_name}.`
			)
		)
			return;
		try {
			await api.post(`/api/links/${link.id}/unlink`);
			await Promise.all([loadFiles(), loadLinks()]);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to unlink';
			console.error('Failed to unlink:', err);
		}
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
	}

	function handleDragEnter(e: DragEvent) {
		e.preventDefault();
		dragCounter++;
		isDragging = true;
	}

	function handleDragLeave(e: DragEvent) {
		e.preventDefault();
		dragCounter--;
		if (dragCounter <= 0) {
			dragCounter = 0;
			isDragging = false;
		}
	}

	interface UploadItem {
		file: File;
		path: string;
	}

	async function handleDrop(e: DragEvent) {
		e.preventDefault();
		dragCounter = 0;
		isDragging = false;

		const dataTransfer = e.dataTransfer;
		if (!dataTransfer) return;

		const items = dataTransfer.items;
		if (items && items.length > 0 && typeof (items[0] as any).webkitGetAsEntry === 'function') {
			const collected: UploadItem[] = [];
			for (let i = 0; i < items.length; i++) {
				const entry = (items[i] as any).webkitGetAsEntry();
				if (entry) {
					await collectFilesFromEntry(entry, '', collected);
				}
			}
			if (collected.length > 0) {
				await uploadFiles(collected);
				return;
			}
		}

		const files = dataTransfer.files;
		if (files && files.length > 0) {
			await uploadFiles(files);
		}
	}

	async function collectFilesFromEntry(entry: any, path: string, items: UploadItem[]): Promise<void> {
		if (entry.isFile) {
			await new Promise<void>((resolve) => {
				entry.file(
					(file: File) => {
						items.push({ file, path: path + file.name });
						resolve();
					},
					() => resolve()
				);
			});
		} else if (entry.isDirectory) {
			const reader = entry.createReader();
			await new Promise<void>((resolve) => {
				const readBatch = () => {
					reader.readEntries(async (entries: any[]) => {
						if (entries.length === 0) {
							resolve();
							return;
						}
						for (const e of entries) {
							await collectFilesFromEntry(e, path + entry.name + '/', items);
						}
						readBatch();
					}, () => resolve());
				};
				readBatch();
			});
		}
	}

	async function uploadFiles(fileList: FileList | UploadItem[]) {
		uploadLoading = true;
		error = '';

		try {
			if (fileList instanceof FileList) {
				for (let i = 0; i < fileList.length; i++) {
					const file = fileList[i];
					const formData = new FormData();
					const relativePath = (file as any).webkitRelativePath || '';
					formData.append('file', file);
					if (relativePath) {
						formData.append('relative_path', relativePath);
					}
					
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
			} else {
				for (const item of fileList) {
					const formData = new FormData();
					formData.append('file', item.file);
					formData.append('relative_path', item.path);
					
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

	function handleFolderPicker() {
		const input = document.createElement('input');
		input.type = 'file';
		(input as any).webkitdirectory = true;
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

	$: pathSegments = currentPath.split('/').filter(Boolean);
</script>

<svelte:window on:keydown={handleKeydown} on:paste={handlePasteEvent} />

{#if editingFile}
	<div class="editor-container" class:mobile={$inputMode === 'mobile'}>
		<div class="editor-header">
			<div class="filename-container">
				<span class="filename">{selectedFile?.name}</span>
				{#if isGzipped}
					<span class="badge badge-warning">Read-only (Compressed)</span>
				{/if}
			</div>
			<div class="editor-actions">
				<button class="btn" on:click={saveFile} disabled={loading || isGzipped || isImageFileOpen}>Save</button>
				<button class="btn secondary" on:click={closeEditor}>Cancel</button>
			</div>
		</div>
		{#if isImageFileOpen}
			<div class="image-viewer">
				<img src={imageUrl} alt={selectedFile?.name} />
			</div>
		{:else if isGzipped}
			<textarea bind:value={fileContent} class="editor" disabled={loading || isGzipped} readonly></textarea>
		{:else}
			<AceEditor
				bind:value={fileContent}
				language={getAceMode(selectedFile?.name || '')}
				readonly={false}
			/>
		{/if}
	</div>
{:else}
	<div 
		class="file-browser"
		class:mobile={$inputMode === 'mobile'}
		class:dragging={isDragging}
		on:dragenter={handleDragEnter}
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
			<button class="btn" on:click={pasteFromClipboardButton} title="Paste a copied file link here (or Ctrl+V)">
				Paste Link
			</button>
			<button class="btn" on:click={handleFilePicker} disabled={uploadLoading}>
				{uploadLoading ? 'Uploading...' : 'Upload'}
			</button>
				<button class="btn" on:click={handleFolderPicker} disabled={uploadLoading}>
					Upload Folder
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
				{@const link = linkFor(file)}
				{@const effDir = file.is_dir || link?.source_is_dir}
				<div class="file-item" class:folder={effDir} class:compact={$inputMode === 'mobile'}>
					<span class="col-icon">
						{#if effDir}📁{:else}📄{/if}
					</span>
					<span class="col-name" on:click={() => openFile(file)}>
						{file.name}
						{#if link}
							<span
								class="link-badge"
								class:missing={link.source_missing}
								title={link.source_missing
									? `Linked from ${link.source_name} — source missing!`
									: `Linked from ${link.source_name} (${link.source_path})`}
							>🔗</span>
						{/if}
					</span>
					{#if $inputMode === 'desktop'}
						<span class="col-size">{effDir ? '-' : formatSize(file.size)}</span>
						<span class="col-modified">{formatDate(file.modified, $globalSettings.server_timezone)}</span>
					{/if}
					<span class="col-actions">
						{#if link}
							<button class="action-btn" on:click={() => unlinkLocalCopy(link)} title="Unlink (keep a local copy here)">⧉</button>
						{:else}
							<button class="action-btn" on:click={() => copyLink(file)} title="Copy link (paste in another server to share)">🔗</button>
						{/if}
						{#if !effDir}
							<button class="action-btn" on:click={() => downloadFile(file)} title="Download">⬇</button>
						{/if}
						<button class="action-btn" on:click={() => openRenameModal(file)} title="Rename">✎</button>
						<button class="action-btn danger" on:click={() => deleteFile(file)} title={link ? 'Remove link' : 'Delete'}>🗑</button>
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

{#if showPasteModal && pendingPaste}
	<div class="modal-overlay" on:click={() => (showPasteModal = false)}>
		<div class="modal" on:click|stopPropagation>
			<h3>Link File Here</h3>
			<p class="paste-info">
				Link <strong>{pendingPaste.p.split('/').pop()}</strong> from
				<strong>{pendingPaste.t} {pendingPaste.n || pendingPaste.id}</strong> into this folder?
			</p>
			<p class="paste-detail">
				Both servers will read and write the <em>same data</em>. It appears here after the next
				start/restart. Avoid pointing two running servers at flatfile data — that can corrupt it.
			</p>
			<div class="modal-actions">
				<button class="btn" on:click={confirmPaste} disabled={pasteLoading}>
					{pasteLoading ? 'Linking...' : 'Link It'}
				</button>
				<button class="btn secondary" on:click={() => (showPasteModal = false)} disabled={pasteLoading}>Cancel</button>
			</div>
		</div>
	</div>
{/if}

{#if linkToast}
	<div class="link-toast">{linkToast}</div>
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
		border-radius: 0;
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
		border-bottom: 3px solid var(--border);
		font-size: 0.75rem;
		font-weight: 600;
		color: var(--text-secondary);
		text-transform: uppercase;
	}

	.file-item {
		display: grid;
		grid-template-columns: 40px 1fr 100px 140px 120px;
		padding: 0.75rem 1.5rem;
		border-bottom: 3px solid var(--border);
		align-items: center;
		transition: background-color 0.2s;
	}

	.file-item:hover {
		background-color: var(--bg-secondary);
	}

	.file-item.compact {
		grid-template-columns: 32px 1fr auto;
		padding: 0.625rem 1rem;
	}

	.col-icon {
		font-size: 1.125rem;
	}

	.compact .col-icon {
		font-size: 1rem;
	}

	.col-name {
		color: var(--text-primary);
		font-weight: 500;
		cursor: pointer;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
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
		border-radius: 0;
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
		border: 6px dashed var(--accent);
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.25rem;
		color: var(--accent);
		z-index: 10;
		pointer-events: none;
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
		border-bottom: 3px solid var(--border);
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
		border-radius: 0;
		font-size: 0.75rem;
		font-weight: 500;
	}

	.badge-warning {
		background-color: rgba(245, 158, 11, 0.1);
		color: #f59e0b;
		border: 3px solid rgba(245, 158, 11, 0.3);
	}

	.link-badge {
		margin-left: 0.5rem;
		font-size: 0.75rem;
		cursor: help;
	}

	.link-badge.missing {
		filter: hue-rotate(-90deg);
	}

	.link-toast {
		position: fixed;
		bottom: 1.5rem;
		left: 50%;
		transform: translateX(-50%);
		background: var(--bg-secondary);
		border: 3px solid var(--accent);
		color: var(--text-primary);
		padding: 0.75rem 1.25rem;
		font-size: 0.875rem;
		z-index: 2000;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
	}

	.paste-info {
		color: var(--text-primary);
		margin: 0 0 0.75rem;
	}

	.paste-detail {
		color: var(--text-secondary);
		font-size: 0.8125rem;
		margin: 0 0 1rem;
		line-height: 1.4;
	}

	.editor-actions {
		display: flex;
		gap: 0.5rem;
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

	.image-viewer {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		width: 100%;
		background: #0a0a0a;
	}

	.image-viewer img {
		width: 60vw;
		height: 60vh;
		object-fit: contain;
		image-rendering: pixelated;
		image-rendering: crisp-edges;
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
		border: 3px solid var(--border);
		padding: 1.5rem;
		border-radius: 0;
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
		border: 3px solid var(--border);
		border-radius: 0;
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

	.mobile .toolbar {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
	}

	.mobile .file-header {
		display: none;
	}

	.mobile .editor-header {
		flex-direction: column;
		align-items: flex-start;
		gap: 0.75rem;
		padding: 0.75rem 1rem;
	}

	.mobile .image-viewer img {
		width: 90vw;
		height: 70vh;
	}

	.mobile .modal {
		min-width: auto;
		margin: 1rem;
	}
</style>
