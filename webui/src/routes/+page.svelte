<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { servers, loadServers, updateServerPosition } from '$lib/stores/servers';
	import { api, type Server } from '$lib/api';
	import ServerTile from '$lib/components/ServerTile.svelte';
	import CreateServer from '$lib/components/CreateServer.svelte';

	let canvasOffset = { x: 0, y: 0 };
	let zoom = 1;
	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let showCreateModal = false;

	$: serverList = $servers || [];

	onMount(() => {
		loadServers();
	});

	function handleWheel(e: WheelEvent) {
		e.preventDefault();
		const delta = e.deltaY > 0 ? -0.1 : 0.1;
		zoom = Math.max(0.5, Math.min(2.0, zoom + delta));
	}

	function handleMouseDown(e: MouseEvent) {
		if ((e.target as HTMLElement).closest('.server-tile')) {
			return;
		}
		isDragging = true;
		dragStart = { x: e.clientX - canvasOffset.x, y: e.clientY - canvasOffset.y };
		(e.target as HTMLElement).style.cursor = 'grabbing';
	}

	function handleMouseMove(e: MouseEvent) {
		if (!isDragging) return;
		canvasOffset = {
			x: e.clientX - dragStart.x,
			y: e.clientY - dragStart.y
		};
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

	function getCanvasTransform() {
		return `translate(${canvasOffset.x}px, ${canvasOffset.y}px) scale(${zoom})`;
	}
</script>

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
	<div class="canvas" style="transform: {getCanvasTransform()}">
		<!-- Connection Lines -->
		<svg class="connection-lines">
			{#each serverList as server}
				{#if server.proxy_id}
					{@const proxy = serverList.find(p => p.id === server.proxy_id)}
					{#if proxy}
						<line 
							x1={proxy.canvas_x + 60}
							y1={proxy.canvas_y + 60}
							x2={server.canvas_x + 60}
							y2={server.canvas_y + 60}
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
				on:click={() => handleServerClick(server)}
				on:move={(e) => handleServerMove(server, e.detail.x, e.detail.y)}
			/>
		{/each}
	</div>

<!-- Create Button -->
	<button class="create-btn" on:click={() => showCreateModal = true}>
		Create+
	</button>
</main>

<CreateServer bind:show={showCreateModal} />

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
		width: 100%;
		height: 100%;
		transform-origin: 0 0;
	}

	.connection-lines {
		position: absolute;
		top: 0;
		left: 0;
		width: 100%;
		height: 100%;
		pointer-events: none;
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
		@apply bg-bg-secondary border border-border;
		padding: 2rem;
		border-radius: 0.5rem;
		max-width: 500px;
		width: 90%;
	}

	.modal h2 {
		margin-top: 0;
		@apply text-text-primary;
	}
</style>
