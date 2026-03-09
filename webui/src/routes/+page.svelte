<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { servers, loadServers, updateServerPosition, updateServerStatus, deleteServerFromStore } from '$lib/stores/servers';
	import { api, type Server } from '$lib/api';
	import ServerTile from '$lib/components/ServerTile.svelte';
	import CreateServer from '$lib/components/CreateServer.svelte';

	let canvasOffset = { x: 0, y: 0 };
	let zoom = 1;
	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let showCreateModal = false;
	let contextMenuServer: Server | null = null;
	let contextMenuPos = { x: 0, y: 0 };
	let pendingDelete = false;
	let ignoreNextClick = false;

	$: serverList = $servers || [];

	onMount(() => {
		loadServers();
		const navbarHeight = 56;
		canvasOffset = {
			x: window.innerWidth / 2 - 4000,
			y: (window.innerHeight - navbarHeight) / 2 - 4000
		};
	});

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		
		const mouseX = (e.clientX - canvasOffset.x) / zoom;
		const mouseY = (e.clientY - canvasOffset.y) / zoom;
		
		const delta = e.deltaY > 0 ? -0.1 : 0.1;
		const newZoom = Math.max(0.1, Math.min(4.0, zoom + delta));
		
		canvasOffset = {
			x: e.clientX - mouseX * newZoom,
			y: e.clientY - mouseY * newZoom
		};
		zoom = newZoom;
	}

	function handleMouseDown(e: MouseEvent) {
		const target = e.target as HTMLElement;
		const isInsideTile = target.closest('.server-tile');
		
		if (isInsideTile) {
			return;
		}
		isDragging = true;
		dragStart = { x: e.clientX - canvasOffset.x, y: e.clientY - canvasOffset.y };
		target.style.cursor = 'grabbing';
	}

	function handleMouseMove(e: MouseEvent) {
		if (!isDragging) {
			return;
		}
		canvasOffset = { x: e.clientX - dragStart.x, y: e.clientY - dragStart.y };
	}

	function handleMouseUp(e: MouseEvent) {
		isDragging = false;
		(e.target as HTMLElement).style.cursor = 'grab';
	}

	function handleServerClick(server: Server) {
		goto(`/servers/${server.id}`);
	}

	async function handleServerMove(server: Server, newX: number, newY: number) {
		await updateServerPosition(server.id, newX, newY);
	}

	async function handleServerStart(server: Server) {
		try {
			await api.post(`/api/servers/${server.id}/start`);
			updateServerStatus(server.id, 'running');
		} catch (err) {
			console.error('Failed to start server:', err);
		}
	}

	async function handleServerStop(server: Server) {
		try {
			await api.post(`/api/servers/${server.id}/stop`);
			updateServerStatus(server.id, 'stopped');
		} catch (err) {
			console.error('Failed to stop server:', err);
		}
	}

	async function handleServerDelete(server: Server) {
		try {
			await api.delete(`/api/servers/${server.id}`);
			deleteServerFromStore(server.id);
		} catch (err) {
			console.error('Failed to delete server:', err);
		}
	}

	function handleContextMenu(server: Server, x: number, y: number) {
		contextMenuServer = server;
		contextMenuPos = { x, y };
		pendingDelete = false;
		ignoreNextClick = true;
	}

	function closeContextMenu() {
		contextMenuServer = null;
		pendingDelete = false;
	}

	function handleContextMenuAction(action: 'start' | 'stop' | 'delete') {
		if (action === 'delete' && !pendingDelete) {
			pendingDelete = true;
			return;
		}
		if (contextMenuServer) {
			if (action === 'start') handleServerStart(contextMenuServer);
			if (action === 'stop') handleServerStop(contextMenuServer);
			if (action === 'delete') handleServerDelete(contextMenuServer);
		}
		closeContextMenu();
	}

	function handleGlobalClick(e: MouseEvent) {
		if (ignoreNextClick) {
			ignoreNextClick = false;
			return;
		}
		if (contextMenuServer && !(e.target as HTMLElement).closest('.context-menu')) {
			closeContextMenu();
		}
	}

	function handleGlobalKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') closeContextMenu();
	}

	function getCanvasTransform() {
		return `translate(${canvasOffset.x}px, ${canvasOffset.y}px) scale(${zoom})`;
	}
</script>

<svelte:window on:click={handleGlobalClick} on:keydown={handleGlobalKeydown} />

<svelte:head>
	<title>Servers - DemiMine</title>
</svelte:head>

<main class="canvas-container" 
	on:wheel={handleWheel}
	on:mousedown={handleMouseDown}
	on:mousemove={handleMouseMove}
	on:mouseup={handleMouseUp}
	on:mouseleave={handleMouseUp}
>
	<div class="canvas" style="transform: translate({canvasOffset.x}px, {canvasOffset.y}px) scale({zoom})">
		<!-- Visual Grid for debugging -->
		<div class="grid-background"></div>
		
		<!-- Connection Lines -->
		<svg class="connection-lines">
			{#each serverList as server}
				{#if server.proxy_id}
					{@const proxy = serverList.find(p => p.id === server.proxy_id)}
					{#if proxy}
						<line 
							x1={proxy.canvas_x + 50}
							y1={proxy.canvas_y + 50}
							x2={server.canvas_x + 50}
							y2={server.canvas_y + 50}
							stroke="#4a5568"
							stroke-width="2"
							stroke-dasharray="5,5"
						/>
					{/if}
				{/if}
			{/each}
		</svg>

		<!-- Server Tiles -->
		{#each serverList as server}
			<ServerTile 
				{server}
				{canvasOffset}
				{zoom}
				on:click={() => handleServerClick(server)}
				on:move={(e) => handleServerMove(server, e.detail.x, e.detail.y)}
				on:contextmenu={(e) => handleContextMenu(server, e.detail.x, e.detail.y)}
			/>
		{/each}
	</div>

<!-- Create Button -->
	<button class="create-btn" on:click={() => showCreateModal = true}>
		Create+
	</button>
</main>

<CreateServer bind:show={showCreateModal} />

{#if contextMenuServer}
	<div 
		class="context-menu" 
		style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
	>
		{#if contextMenuServer.status === 'running'}
			<button class="menu-item" on:click={() => handleContextMenuAction('stop')}>
				Stop
			</button>
		{:else}
			<button class="menu-item" on:click={() => handleContextMenuAction('start')}>
				Start
			</button>
		{/if}
		<button 
			class="menu-item danger" 
			class:confirm={pendingDelete}
			on:click={() => handleContextMenuAction('delete')}
		>
			{pendingDelete ? 'Click again to confirm' : 'Delete'}
		</button>
	</div>
{/if}

<style>
	.canvas-container {
		position: fixed;
		top: 56px;
		left: 0;
		right: 0;
		bottom: 0;
		overflow: hidden;
		cursor: grab;
		background: var(--bg-primary);
	}

	.canvas {
		position: absolute;
		width: 8000px;
		height: 8000px;
		transform-origin: 0 0;
		z-index: 1;
	}

	.grid-background {
		position: absolute;
		top: 0;
		left: 0;
		width: 8000px;
		height: 8000px;
		background-image: 
			linear-gradient(rgba(74, 85, 104, 0.2) 1px, transparent 1px),
			linear-gradient(90deg, rgba(74, 85, 104, 0.2) 1px, transparent 1px);
		background-size: 100px 100px;
		pointer-events: none;
	}

	.connection-lines {
		position: absolute;
		top: 0;
		left: 0;
		width: 8000px;
		height: 8000px;
		pointer-events: none;
		z-index: 2;
	}

	.create-btn {
		position: fixed;
		left: 24px;
		top: 80px;
		@apply bg-accent hover:bg-accent-hover text-text-primary;
		padding: 0.75rem 1.5rem;
		border-radius: 0.5rem;
		font-weight: 600;
		box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
		z-index: 10;
	}

	.context-menu {
		position: fixed;
		background-color: var(--bg-secondary);
		border: 1px solid var(--border);
		border-radius: 0.375rem;
		padding: 0.25rem;
		z-index: 1000;
		min-width: 140px;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
	}

	.menu-item {
		display: block;
		width: 100%;
		padding: 0.5rem 0.75rem;
		background: transparent;
		border: none;
		color: var(--text-primary);
		text-align: left;
		cursor: pointer;
		border-radius: 0.25rem;
		font-size: 0.875rem;
		transition: background-color 0.15s;
	}

	.menu-item:hover {
		background-color: var(--bg-tertiary);
	}

	.menu-item.danger {
		color: var(--error);
	}

	.menu-item.danger:hover {
		background-color: rgba(239, 68, 68, 0.1);
	}

	.menu-item.confirm {
		font-weight: 500;
	}
</style>
