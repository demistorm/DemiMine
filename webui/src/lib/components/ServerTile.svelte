<script lang="ts">
	import type { Server } from '$lib/api';
	import { createEventDispatcher, onDestroy } from 'svelte';

	export let server: Server;
	export let canvasOffset = { x: 0, y: 0 };
	export let zoom = 1;

	const dispatch = createEventDispatcher();

	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let currentX = server.canvas_x ?? 0;
	let currentY = server.canvas_y ?? 0;

	$: if (!isDragging && server.canvas_x != null) currentX = server.canvas_x;
	$: if (!isDragging && server.canvas_y != null) currentY = server.canvas_y;

	function handleMouseDown(e: MouseEvent) {
        if (!e.shiftKey) {
            dispatch('click');
            return;
        }

        e.preventDefault();
        e.stopPropagation();
        isDragging = true;
        dragStart = {
            x: (e.clientX - canvasOffset.x) / zoom - currentX,
            y: (e.clientY - canvasOffset.y) / zoom - currentY
        };
        window.addEventListener('mousemove', handleMouseMove);
        window.addEventListener('mouseup', handleMouseUp);
    }

	function handleMouseMove(e: MouseEvent) {
        if (!isDragging) return;

        e.preventDefault();
        const newX = (e.clientX - canvasOffset.x) / zoom - dragStart.x;
        const newY = (e.clientY - canvasOffset.y) / zoom - dragStart.y;

        currentX = Math.round(newX / 100) * 100;
        currentY = Math.round(newY / 100) * 100;
    }

	function handleMouseUp(e: MouseEvent) {
        if (isDragging) {
            dispatch('move', { x: currentX, y: currentY });
        }
        isDragging = false;
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', handleMouseUp);
    }

	onDestroy(() => {
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', handleMouseUp);
    });

	function getStatusColor(status: string) {
        switch (status) {
            case 'running':
                return 'var(--success)';
            case 'stopped':
                return 'var(--error)';
            default:
                return 'var(--text-secondary)';
        }
    }

	function getStatusText(status: string) {
        switch (status) {
            case 'running':
                return 'Online';
            case 'stopped':
                return 'Offline';
            case 'starting':
                return 'Starting';
            case 'crashed':
                return 'Crashed';
            default:
                return status;
        }
    }
</script>

<svelte:head>
    <title>{server.name} - DemiMine</title>
</svelte:head>

<div
    class="server-tile"
    class:dragging={isDragging}
    style="left: {currentX + 50}px; top: {currentY + 50}px; transform: translate(-50%, -50%);"
    on:mousedown={handleMouseDown}
    role="button"
    tabindex={0}
>
    <div class="server-icon">
        {#if server.icon_path}
            <img src={server.icon_path} alt={server.name} />
        {:else}
            <div class="icon-placeholder">
                Icon
            </div>
        {/if}
    </div>
    
    <div class="server-info">
        <div class="server-name">{server.name}</div>
        <div class="server-status">
            <span class="status-dot" style="background-color: {getStatusColor(server.status)}"></span>
            <span class="status-text">{getStatusText(server.status)}</span>
            {#if server.status === 'running'}
                <span class="player-count">({server.player_count})</span>
            {/if}
        </div>
    </div>
</div>

<style>
    .server-tile {
        position: absolute;
        width: 120px;
        padding: 1rem;
        background-color: var(--bg-secondary);
        border: 1px solid var(--border);
        border-radius: 0.5rem;
        cursor: pointer;
        transition: transform 0.2s, box-shadow 0.2s;
        user-select: none;
    }

    .server-tile:hover {
        transform: translate(-50%, calc(-50% - 2px));
        box-shadow: 0 8px 16px rgba(0, 0, 0, 0.3);
    }

    .server-tile.dragging {
        opacity: 0.7;
        cursor: move;
        z-index: 10;
        transform: translate(-50%, -50%);
    }

    .server-icon {
        width: 64px;
        height: 64px;
        margin: 0 auto 0.5rem;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    .server-icon img {
        width: 100%;
        height: 100%;
        object-fit: contain;
    }

    .icon-placeholder {
        background-color: var(--bg-tertiary);
        color: var(--text-secondary);
        width: 100%;
        height: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
        border-radius: 0.25rem;
    }

    .server-info {
        text-align: center;
        margin-top: 0.5rem;
    }

    .server-name {
        font-weight: 600;
        color: var(--text-primary);
        margin-bottom: 0.25rem;
    }

    .server-status {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 0.5rem;
        font-size: 0.875rem;
    }

    .status-dot {
        width: 8px;
        height: 8px;
        border-radius: 50%;
    }

    .status-text {
        color: var(--text-secondary);
        font-weight: 500;
    }

    .player-count {
        color: var(--text-secondary);
        font-size: 0.875rem;
    }
</style>
