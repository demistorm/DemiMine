<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';
	import { ws } from '$lib/websocket';
	import { AnsiUp } from 'ansi_up';

	export let serverId: number;

	let logs: string[] = [];
	let command = '';
	let commandHistory: string[] = [];
	let historyIndex = -1;
	let autoScroll = true;
	let logContainer: HTMLElement;
	let connected = false;
	let error: string | null = null;
	let ansiUp: AnsiUp;
	const MAX_LOG_LINES = 1000;

	function renderLog(line: string): string {
		if (!ansiUp) return line;
		try {
			return ansiUp.ansi_to_html(line);
		} catch (e) {
			console.error('Failed to convert ANSI to HTML:', e);
			return line;
		}
	}

	onMount(async () => {
		ansiUp = new AnsiUp();
		ansiUp.use_classes = true;

		await loadLogs();
		await loadCommandHistory();
		scrollToBottom();

		await ws.connect();
		ws.subscribe(`server:${serverId}`);
		connected = ws.isConnected();

		ws.on('log', handleMessage);
	});

	onDestroy(() => {
		ws.off('log', handleMessage);
		ws.unsubscribe(`server:${serverId}`);
	});

	function handleMessage(message: any) {
		if (message.server_id !== serverId) return;

		switch (message.type) {
			case 'log':
				if (message.log_line) {
					const rendered = renderLog(message.log_line);
					logs = [...logs, rendered];
					if (logs.length > MAX_LOG_LINES) {
						logs = logs.slice(-MAX_LOG_LINES);
					}
					scrollToBottom();
				}
				break;
			case 'error':
				if (message.error) {
					error = message.error;
					console.error('Server error:', message.error);
					setTimeout(() => error = null, 5000);
				}
				break;
		}
	}

	async function loadLogs() {
		try {
			const response = await api.get<{ logs: string[] }>(`/api/servers/${serverId}/logs?lines=400`);
			const rawLogs = response.logs || [];
			logs = rawLogs.map(renderLog);
		} catch (error) {
			console.error('Failed to load logs:', error);
		}
	}

	async function loadCommandHistory() {
		try {
			const response = await api.get<{ commands: { command: string; executed_at: string }[] }>(`/api/servers/${serverId}/history?limit=50`);
			commandHistory = (response.commands || []).map(c => c.command);
		} catch (error) {
			console.error('Failed to load command history:', error);
		}
	}

	async function submitCommand() {
		if (!command.trim()) return;

		const commandToSend = command;

		if (ws.isConnected()) {
			console.log('Sending command via WebSocket:', commandToSend);
			try {
				ws.sendCommand(serverId, commandToSend);
				commandHistory = [commandToSend, ...commandHistory];
				command = '';
				historyIndex = -1;
				error = null;
			} catch (err) {
				console.error('Failed to send command via WebSocket:', err);
				error = 'Failed to send command';
				setTimeout(() => error = null, 5000);
			}
		} else {
			console.log('Sending command via HTTP API:', commandToSend);
			try {
				await api.post(`/api/servers/${serverId}/command`, { command: commandToSend });
				commandHistory = [commandToSend, ...commandHistory];
				command = '';
				historyIndex = -1;
				error = null;
				await loadLogs();
				scrollToBottom();
			} catch (err: any) {
				console.error('Failed to send command via HTTP:', err);
				error = err?.error || 'Failed to send command';
				setTimeout(() => error = null, 5000);
			}
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			submitCommand();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			if (historyIndex < commandHistory.length - 1) {
				historyIndex++;
				command = commandHistory[historyIndex];
			}
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			if (historyIndex > 0) {
				historyIndex--;
				command = commandHistory[historyIndex];
			} else {
				historyIndex = -1;
				command = '';
			}
		}
	}

	function scrollToBottom() {
		if (logContainer && autoScroll) {
			setTimeout(() => {
				logContainer.scrollTop = logContainer.scrollHeight;
			}, 0);
		}
	}

	function handleScroll() {
		if (!logContainer) return;
		const { scrollTop, scrollHeight, clientHeight } = logContainer;
		autoScroll = scrollTop + clientHeight >= scrollHeight - 10;
	}
</script>

<div class="console-container">
	<div class="log-output" bind:this={logContainer} on:scroll={handleScroll}>
		{#if logs.length === 0}
			<div class="empty-state">No logs available. Start the server to see console output.</div>
		{:else}
			{#each logs as log, index}
				<div class="log-line" class:last={index === logs.length - 1}>
					<span class="log-text" class:last={index === logs.length - 1}>{@html log}</span>
				</div>
			{/each}
		{/if}
	</div>

	{#if error}
		<div class="error-message">
			{error}
		</div>
	{/if}

	<div class="command-input">
		<div class="connection-status">
			<span class="status-dot" class:connected class:disconnected={!connected}></span>
			<span class="status-text">{connected ? 'Live' : 'Polling'}</span>
		</div>
		<input 
			type="text" 
			bind:value={command}
			on:keydown={handleKeydown}
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
		font-family: 'Courier New', Courier, monospace;
		font-size: 0.8125rem;
		background-color: #0a0a0a;
	}

	.log-line {
		padding: 0.125rem 0;
		color: var(--text-primary);
		white-space: pre-wrap;
		word-wrap: break-word;
	}

	.log-text {
		line-height: 1.4;
	}

	.empty-state {
		color: var(--text-secondary);
		text-align: center;
		padding: 2rem;
	}

	.command-input {
		display: flex;
		gap: 0.5rem;
		padding: 1rem;
		border-top: 1px solid var(--border);
		background-color: var(--bg-secondary);
		align-items: center;
	}

	.connection-status {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding-right: 0.75rem;
		border-right: 1px solid var(--border);
	}

	.status-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background-color: var(--text-secondary);
	}

	.status-dot.connected {
		background-color: var(--success);
	}

	.status-text {
		font-size: 0.75rem;
		color: var(--text-secondary);
		min-width: 50px;
	}

	.command-input input {
		flex: 1;
		background-color: var(--bg-primary);
		border: 1px solid var(--border);
		color: var(--text-primary);
		padding: 0.625rem 0.875rem;
		border-radius: 0.375rem;
		font-family: 'Courier New', Courier, monospace;
	}

	.command-input input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.command-input button {
		background-color: var(--accent);
		color: var(--text-primary);
		border: none;
		padding: 0.625rem 1.25rem;
		border-radius: 0.375rem;
		cursor: pointer;
		font-weight: 500;
	}

	.command-input button:hover:not(:disabled) {
		background-color: var(--accent-hover);
	}

	.command-input button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.error-message {
		padding: 0.75rem 1rem;
		background-color: rgba(239, 68, 68, 0.1);
		border: 1px solid rgba(239, 68, 68, 0.3);
		border-radius: 0.375rem;
		color: #ef4444;
		font-size: 0.875rem;
		margin: 0 1rem 0.5rem 1rem;
		animation: fadeIn 0.3s ease-in;
	}

	@keyframes fadeIn {
		from {
			opacity: 0;
			transform: translateY(-5px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	/* ANSI color classes for terminal output */
	.ansi-black-fg { color: #000000; }
	.ansi-red-fg { color: #cd3131; }
	.ansi-green-fg { color: #0dbc79; }
	.ansi-yellow-fg { color: #e5e510; }
	.ansi-blue-fg { color: #2472c8; }
	.ansi-magenta-fg { color: #bc3fbc; }
	.ansi-cyan-fg { color: #11a8cd; }
	.ansi-white-fg { color: #e5e5e5; }
	.ansi-bright-black-fg { color: #666666; }
	.ansi-bright-red-fg { color: #f14c4c; }
	.ansi-bright-green-fg { color: #23d18b; }
	.ansi-bright-yellow-fg { color: #f5f543; }
	.ansi-bright-blue-fg { color: #3b8eea; }
	.ansi-bright-magenta-fg { color: #d670d6; }
	.ansi-bright-cyan-fg { color: #29b8db; }
	.ansi-bright-white-fg { color: #ffffff; }

	.ansi-black-bg { background-color: #000000; }
	.ansi-red-bg { background-color: #cd3131; }
	.ansi-green-bg { background-color: #0dbc79; }
	.ansi-yellow-bg { background-color: #e5e510; }
	.ansi-blue-bg { background-color: #2472c8; }
	.ansi-magenta-bg { background-color: #bc3fbc; }
	.ansi-cyan-bg { background-color: #11a8cd; }
	.ansi-white-bg { background-color: #e5e5e5; }
	.ansi-bright-black-bg { background-color: #666666; }
	.ansi-bright-red-bg { background-color: #f14c4c; }
	.ansi-bright-green-bg { background-color: #23d18b; }
	.ansi-bright-yellow-bg { background-color: #f5f543; }
	.ansi-bright-blue-bg { background-color: #3b8eea; }
	.ansi-bright-magenta-bg { background-color: #d670d6; }
	.ansi-bright-cyan-bg { background-color: #29b8db; }
	.ansi-bright-white-bg { background-color: #ffffff; }

	.ansi-bold { font-weight: bold; }
	.ansi-italic { font-style: italic; }
	.ansi-underline { text-decoration: underline; }
	.ansi-blink { animation: blink 1s step-end infinite; }
	.ansi-inverse { filter: invert(1); }

	@keyframes blink {
		50% { opacity: 0; }
	}
</style>
