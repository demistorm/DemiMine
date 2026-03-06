<script lang="ts">
	import type { Server } from '$lib/api';

	export let server: Server;

	let ram = server.ram_mb || 2048;
	let autoShutdown = server.auto_shutdown_minutes || 15;
</script>

<div class="settings">
	<div class="section">
		<h2>General Settings</h2>
		
		<div class="field">
			<label for="name">Server Name</label>
			<input type="text" id="name" value={server.name} />
		</div>

		<div class="field">
			<label for="ram">RAM Allocation (MB)</label>
			<input type="number" id="ram" bind:value={ram} />
		</div>

		<div class="field">
			<label for="shutdown">Auto-shutdown after (minutes)</label>
			<input type="number" id="shutdown" bind:value={autoShutdown} />
		</div>

		<div class="field">
			<label>Server Icon</label>
			<div class="icon-upload">
				{#if server.icon_path}
					<img src={server.icon_path} alt={server.name} />
				{:else}
					<div class="icon-placeholder">No icon</div>
				{/if}
				<button class="btn">Upload Icon</button>
			</div>
		</div>
	</div>

	<div class="section">
		<h2>Network</h2>
		
		<div class="field">
			<label for="port">Host Port</label>
			<input type="number" id="port" value={server.host_port || ''} />
		</div>

		<div class="field">
			<label for="proxy">Proxy Assignment</label>
			<select id="proxy">
				<option value="">Standalone (no proxy)</option>
			</select>
		</div>
	</div>

	<div class="section">
		<h2>Actions</h2>
		
		<div class="actions">
			<button class="btn">Update Server JAR</button>
			<button class="btn btn-danger">Delete Server</button>
		</div>
	</div>

	<div class="save-bar">
		<button class="btn btn-primary">Save Changes</button>
	</div>
</div>

<style>
	.settings {
		max-width: 800px;
	}

	.section {
		margin-bottom: 2.5rem;
	}

	.section h2 {
		color: var(--text-primary);
		font-size: 1.25rem;
		margin-bottom: 1.5rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--border);
	}

	.field {
		margin-bottom: 1.5rem;
	}

	.field label {
		display: block;
		color: var(--text-secondary);
		font-size: 0.875rem;
		margin-bottom: 0.5rem;
		font-weight: 500;
	}

	.field input,
	.field select {
		width: 100%;
		max-width: 400px;
		padding: 0.625rem 0.875rem;
		background-color: var(--bg-secondary);
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

	.icon-upload {
		display: flex;
		align-items: center;
		gap: 1rem;
	}

	.icon-upload img {
		width: 64px;
		height: 64px;
		object-fit: contain;
		border-radius: 0.25rem;
	}

	.icon-placeholder {
		width: 64px;
		height: 64px;
		background-color: var(--bg-tertiary);
		display: flex;
		align-items: center;
		justify-content: center;
		border-radius: 0.25rem;
		color: var(--text-secondary);
		font-size: 0.75rem;
	}

	.actions {
		display: flex;
		gap: 1rem;
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

	.btn:hover {
		background-color: var(--bg-secondary);
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

	.save-bar {
		position: fixed;
		bottom: 0;
		left: 0;
		right: 0;
		padding: 1rem 2rem;
		background-color: var(--bg-secondary);
		border-top: 1px solid var(--border);
		display: flex;
		justify-content: flex-end;
	}
</style>
