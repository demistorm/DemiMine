<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';

	export let serverId: number;

	let logs: string[] = [];
	let command = '';
	let commandHistory: string[] = [];
	let autoScroll = true;

	onMount(async () => {
		await loadLogs();
		await loadCommandHistory();
	});

	async function loadLogs() {
		try {
			const response = await api.get<{ logs: string[] }>(`/api/servers/${serverId}/logs?lines=100`);
			logs = response.logs;
		} catch (error) {
			console.error('Failed to load logs:', error);
		}
	}

	async function loadCommandHistory() {
		try {
			const response = await api.get<{ commands: { command: string; executed_at: string }[] }>(`/api/servers/${serverId}/history?limit=50`);
			commandHistory = response.commands.map(c => c.command);
		} catch (error) {
			console.error('Failed to load command history:', error);
		}
	}

	async function submitCommand() {
		if (!command.trim()) return;
		
		try {
			await api.post(`/api/servers/${serverId}/command`, { command });
			commandHistory = [command, ...commandHistory];
			command = '';
		} catch (error) {
			console.error('Failed to send command:', error);
		}
	}

	function handleScroll(e: UIEvent) {
		const container = e.currentTarget as HTMLElement;
		if (autoScroll) {
			container.scrollTop = container.scrollHeight;
		}
	}
</script>

<div class="console-container">
    <div class="log-output" on:scroll={handleScroll}>
        {#each logs as log}
            <div class="log-line">
                <span class="log-text">{log}</span>
            </div>
        {/each}
    </div>
    
    <div class="command-input">
        <input 
            type="text" 
            bind:value={command}
            on:keydown={(e) => e.key === 'Enter' && submitCommand()}
            placeholder="Enter command..."
        />
        <button on:click={submitCommand} disabled={!command.trim()}>
            Send
        </button>
    </div>
</div>

<style>
    .console-container {
        display: flex;
        flex-direction: column;
        height: 100%;
        background-color: var(--bg-primary);
    }

    .log-output {
        flex: 1;
        overflow-y: auto;
        padding: 1rem;
        font-family: 'Courier New', monospace;
        font-size: 0.875rem;
        background-color: var(--bg-secondary);
        border-radius: 0.25rem;
    }

    .log-line {
        padding: 0.25rem 0;
        color: var(--text-primary);
    }

    .log-text {
        white-space: pre-wrap;
        word-wrap: break-word;
    }

    .command-input {
        display: flex;
        gap: 0.5rem;
        padding: 1rem;
        border-top: 1px solid var(--border);
        background-color: var(--bg-secondary);
    }

    .command-input input {
        flex: 1;
        background-color: var(--bg-primary);
        border: 1px solid var(--border);
        color: var(--text-primary);
        padding: 0.5rem;
        border-radius: 0.25rem;
    }

    .command-input button {
        background-color: var(--accent);
        color: var(--text-primary);
        border: none;
        padding: 0.5rem 1rem;
        border-radius: 0.25rem;
        cursor: pointer;
    }

    .command-input button:hover:not(:disabled) {
        background-color: var(--accent-hover);
    }

    .command-input button:disabled {
        opacity: 0.5;
        cursor: not-allowed;
    }
</style>
