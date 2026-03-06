<script lang="ts">
	export let serverId: number;

	let currentPath = '/';
	let files = [
		{ name: 'server.properties', is_dir: false, size: 1024, modified: '2024-01-15' },
		{ name: 'world', is_dir: true, size: 0, modified: '2024-01-15' },
		{ name: 'plugins', is_dir: true, size: 0, modified: '2024-01-14' },
		{ name: 'logs', is_dir: true, size: 0, modified: '2024-01-15' },
		{ name: 'banned-players.json', is_dir: false, size: 256, modified: '2024-01-10' },
	];

	function navigateTo(path: string) {
		currentPath = path;
	}

	function handleDragOver(e: DragEvent) {
		e.preventDefault();
	}

	function handleDrop(e: DragEvent) {
		e.preventDefault();
		console.log('File upload not implemented yet');
	}
</script>

<div 
	class="file-browser"
	on:dragover={handleDragOver}
	on:drop={handleDrop}
>
	<div class="toolbar">
		<div class="breadcrumb">
			<button on:click={() => navigateTo('/')}>root</button>
			{#each currentPath.split('/').filter(Boolean) as segment}
				<span>/</span>
				<button>{segment}</button>
			{/each}
		</div>
		<div class="actions">
			<button class="btn">Upload</button>
			<button class="btn">New Folder</button>
			<button class="btn">New File</button>
		</div>
	</div>

	<div class="file-list">
		{#each files as file}
			<div class="file-item" class:folder={file.is_dir}>
				<span class="icon">{file.is_dir ? '📁' : '📄'}</span>
				<span class="name">{file.name}</span>
				<span class="size">{file.is_dir ? '-' : file.size + ' B'}</span>
				<span class="modified">{file.modified}</span>
				<div class="file-actions">
					<button>Download</button>
					<button>Delete</button>
				</div>
			</div>
		{/each}
	</div>

	<div class="drop-zone">
		Drop files here to upload
	</div>
</div>

<style>
	.file-browser {
		display: flex;
		flex-direction: column;
		height: 100%;
	}

	.toolbar {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
		padding-bottom: 1rem;
		border-bottom: 1px solid var(--border);
	}

	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.breadcrumb button {
		background: transparent;
		color: var(--text-secondary);
		border: none;
		cursor: pointer;
		font-size: 0.875rem;
		padding: 0.25rem 0.5rem;
		transition: color 0.2s;
	}

	.breadcrumb button:hover {
		color: var(--accent);
	}

	.breadcrumb span {
		color: var(--text-secondary);
	}

	.actions {
		display: flex;
		gap: 0.5rem;
	}

	.btn {
		@apply bg-bg-tertiary hover:bg-accent text-text-primary;
		padding: 0.5rem 1rem;
		border-radius: 0.375rem;
		font-size: 0.875rem;
		border: 1px solid var(--border);
		cursor: pointer;
		transition: all 0.2s;
	}

	.file-list {
		flex: 1;
		overflow-y: auto;
	}

	.file-item {
		display: grid;
		grid-template-columns: 40px 1fr 100px 120px 150px;
		align-items: center;
		padding: 0.75rem 1rem;
		border-bottom: 1px solid var(--border);
		transition: background-color 0.2s;
	}

	.file-item:hover {
		background-color: var(--bg-secondary);
	}

	.file-item.folder .name {
		color: var(--accent);
		cursor: pointer;
	}

	.icon {
		font-size: 1.25rem;
	}

	.name {
		color: var(--text-primary);
		font-weight: 500;
	}

	.size, .modified {
		color: var(--text-secondary);
		font-size: 0.875rem;
	}

	.file-actions {
		display: flex;
		gap: 0.5rem;
		opacity: 0;
		transition: opacity 0.2s;
	}

	.file-item:hover .file-actions {
		opacity: 1;
	}

	.file-actions button {
		background: transparent;
		color: var(--text-secondary);
		border: 1px solid var(--border);
		padding: 0.25rem 0.5rem;
		border-radius: 0.25rem;
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 0.2s;
	}

	.file-actions button:hover {
		color: var(--text-primary);
		background-color: var(--bg-tertiary);
	}

	.drop-zone {
		display: none;
		text-align: center;
		padding: 2rem;
		color: var(--text-secondary);
		border: 2px dashed var(--border);
		border-radius: 0.5rem;
		margin-top: 1rem;
	}

	.file-browser:drag .drop-zone {
		display: block;
	}
</style>
