<script lang="ts">
	export let serverId: number;

	let backups = [
		{ id: 1, name: 'backup-2024-01-15-10-30.zip', size: 524288000, created: '2024-01-15 10:30 AM' },
		{ id: 2, name: 'backup-2024-01-14-10-30.zip', size: 518000000, created: '2024-01-14 10:30 AM' },
		{ id: 3, name: 'backup-2024-01-13-10-30.zip', size: 515000000, created: '2024-01-13 10:30 AM' },
	];

	function formatBytes(bytes: number) {
		const gb = bytes / (1024 * 1024 * 1024);
		const mb = bytes / (1024 * 1024);
		return gb >= 1 ? `${gb.toFixed(2)} GB` : `${mb.toFixed(0)} MB`;
	}

	function createBackup() {
		console.log('Create backup not implemented yet');
	}

	function restoreBackup(id: number) {
		console.log('Restore backup', id, 'not implemented yet');
	}

	function deleteBackup(id: number) {
		console.log('Delete backup', id, 'not implemented yet');
	}
</script>

<div class="backups">
	<div class="header">
		<h2>Backups</h2>
		<button class="btn btn-primary" on:click={createBackup}>
			Create Backup
		</button>
	</div>

	<div class="backup-list">
		{#each backups as backup}
			<div class="backup-item">
				<div class="backup-info">
					<div class="backup-name">{backup.name}</div>
					<div class="backup-meta">
						<span class="size">{formatBytes(backup.size)}</span>
						<span class="separator">•</span>
						<span class="date">{backup.created}</span>
					</div>
				</div>
				<div class="backup-actions">
					<button class="btn" on:click={() => restoreBackup(backup.id)}>Restore</button>
					<button class="btn btn-danger" on:click={() => deleteBackup(backup.id)}>Delete</button>
				</div>
			</div>
		{/each}
	</div>

	{#if backups.length === 0}
		<div class="empty-state">
			<p>No backups yet</p>
			<button class="btn btn-primary" on:click={createBackup}>Create First Backup</button>
		</div>
	{/if}
</div>

<style>
	.backups {
		max-width: 1000px;
	}

	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: 1.5rem;
	}

	.header h2 {
		color: var(--text-primary);
		font-size: 1.5rem;
		margin: 0;
	}

	.backup-list {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.backup-item {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 1.25rem 1.5rem;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 0.5rem;
		transition: border-color 0.2s;
	}

	.backup-item:hover {
		border-color: var(--accent);
	}

	.backup-info {
		flex: 1;
	}

	.backup-name {
		color: var(--text-primary);
		font-weight: 600;
		margin-bottom: 0.5rem;
	}

	.backup-meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		color: var(--text-secondary);
		font-size: 0.875rem;
	}

	.separator {
		color: var(--border);
	}

	.backup-actions {
		display: flex;
		gap: 0.75rem;
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

	.btn:hover {
		background-color: var(--bg-primary);
	}

	.btn-primary {
		background-color: var(--accent);
		border-color: var(--accent);
	}

	.btn-primary:hover {
		background-color: var(--accent-hover);
	}

	.btn-danger {
		color: var(--error);
		border-color: var(--error);
	}

	.btn-danger:hover {
		background-color: var(--error);
		color: var(--text-primary);
	}

	.empty-state {
		text-align: center;
		padding: 3rem;
		color: var(--text-secondary);
	}

	.empty-state p {
		margin-bottom: 1rem;
	}
</style>
