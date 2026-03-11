<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { servers, proxies, loadServers, loadProxies, updateServerPosition, updateServerStatus, deleteServerFromStore, updateProxyPosition, deleteProxyFromStore } from '$lib/stores/servers';
	import { api, type Server, type Proxy } from '$lib/api';
	import ServerTile from '$lib/components/ServerTile.svelte';
	import ProxyTile from '$lib/components/ProxyTile.svelte';
	import CreateServer from '$lib/components/CreateServer.svelte';
	import CreateProxy from '$lib/components/CreateProxy.svelte';

	let canvasOffset = { x: 0, y: 0 };
	let zoom = 1;
	let isDragging = false;
	let dragStart = { x: 0, y: 0 };
	let showCreateModal = false;
	let showCreateProxyModal = false;
	let contextMenuServer: Server | null = null;
	let contextMenuProxy: Proxy | null = null;
	let contextMenuPos = { x: 0, y: 0 };
	let pendingDelete = false;
	let ignoreNextClick = false;

	$: serverList = $servers || [];
	$: proxyList = $proxies || [];

	onMount(() => {
		loadServers();
		loadProxies();
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
        const isInsideTile = target.closest('.server-tile, .proxy-tile');
        
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

    function handleProxyClick(proxy: Proxy) {
        goto(`/proxies/${proxy.id}`);
    }

    async function handleServerMove(server: Server, newX: number, newY: number) {
        await updateServerPosition(server.id, newX, newY);
    }

    async function handleProxyMove(proxy: Proxy, newX: number, newY: number) {
        await updateProxyPosition(proxy.id, newX, newY);
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

    async function handleProxyStart(proxy: Proxy) {
        try {
            await api.post(`/api/proxies/${proxy.id}/start`);
        } catch (err) {
            console.error('Failed to start proxy:', err);
        }
    }

    async function handleProxyStop(proxy: Proxy) {
        try {
            await api.post(`/api/proxies/${proxy.id}/stop`);
        } catch (err) {
            console.error('Failed to stop proxy:', err);
        }
    }

    async function handleProxyDelete(proxy: Proxy) {
        try {
            await api.delete(`/api/proxies/${proxy.id}`);
            deleteProxyFromStore(proxy.id);
        } catch (err) {
            console.error('Failed to delete proxy:', err);
        }
    }

    function handleServerContextMenu(server: Server, x: number, y: number) {
        contextMenuServer = server;
        contextMenuProxy = null;
        contextMenuPos = { x, y };
        pendingDelete = false;
        ignoreNextClick = true;
    }

    function handleProxyContextMenu(proxy: Proxy, x: number, y: number) {
        contextMenuProxy = proxy;
        contextMenuServer = null;
        contextMenuPos = { x, y };
        pendingDelete = false;
        ignoreNextClick = true;
    }

    function closeContextMenu() {
        contextMenuServer = null;
        contextMenuProxy = null;
        pendingDelete = false;
    }

    function handleServerContextMenuAction(action: 'start' | 'stop' | 'delete') {
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

    function handleProxyContextMenuAction(action: 'start' | 'stop' | 'delete') {
        if (action === 'delete' && !pendingDelete) {
            pendingDelete = true;
            return;
        }
        if (contextMenuProxy) {
            if (action === 'start') handleProxyStart(contextMenuProxy);
            if (action === 'stop') handleProxyStop(contextMenuProxy);
            if (action === 'delete') handleProxyDelete(contextMenuProxy);
        }
        closeContextMenu();
    }

    function handleGlobalClick(e: MouseEvent) {
        if (ignoreNextClick) {
            ignoreNextClick = false;
            return;
        }
        if ((contextMenuServer || contextMenuProxy) && !(e.target as HTMLElement).closest('.context-menu')) {
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
        <div class="grid-background"></div>
        
        <svg class="connection-lines">
            {#each proxyList as proxy}
                {#each serverList as server}
                    {#if server.proxy_id === proxy.id}
                        <line 
                            x1={proxy.canvas_x + 60}
                            y1={proxy.canvas_y + 60}
                            x2={server.canvas_x + 60}
                            y2={server.canvas_y + 60}
                            stroke="#63b3ed"
                            stroke-width="2"
                        />
                    {/if}
                {/each}
            {/each}
        </svg>

        {#each proxyList as proxy}
            <ProxyTile 
                {proxy}
                {canvasOffset}
                {zoom}
                on:click={() => handleProxyClick(proxy)}
                on:move={(e) => handleProxyMove(proxy, e.detail.x, e.detail.y)}
                on:contextmenu={(e) => handleProxyContextMenu(proxy, e.detail.x, e.detail.y)}
            />
        {/each}

        {#each serverList as server}
            <ServerTile 
                {server}
                {canvasOffset}
                {zoom}
                on:click={() => handleServerClick(server)}
                on:move={(e) => handleServerMove(server, e.detail.x, e.detail.y)}
                on:contextmenu={(e) => handleServerContextMenu(server, e.detail.x, e.detail.y)}
            />
        {/each}
    </div>

    <div class="create-buttons">
        <button class="create-btn" on:click={() => showCreateModal = true}>
            Create Server
        </button>
        <button class="create-btn proxy" on:click={() => showCreateProxyModal = true}>
            Create Proxy
        </button>
    </div>
</main>

<CreateServer bind:show={showCreateModal} />
<CreateProxy bind:show={showCreateProxyModal} />

{#if contextMenuServer}
    <div 
        class="context-menu" 
        style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
    >
        {#if contextMenuServer.status === 'running'}
            <button class="menu-item" on:click={() => handleServerContextMenuAction('stop')}>
                Stop
            </button>
        {:else}
            <button class="menu-item" on:click={() => handleServerContextMenuAction('start')}>
                Start
            </button>
        {/if}
        <button 
            class="menu-item danger" 
            class:confirm={pendingDelete}
            on:click={() => handleServerContextMenuAction('delete')}
        >
            {pendingDelete ? 'Click again to confirm' : 'Delete'}
        </button>
    </div>
{/if}

{#if contextMenuProxy}
    <div 
        class="context-menu" 
        style="left: {contextMenuPos.x}px; top: {contextMenuPos.y}px;"
    >
        {#if contextMenuProxy.status === 'running'}
            <button class="menu-item" on:click={() => handleProxyContextMenuAction('stop')}>
                Stop
            </button>
        {:else}
            <button class="menu-item" on:click={() => handleProxyContextMenuAction('start')}>
                Start
            </button>
        {/if}
        <button 
            class="menu-item danger" 
            class:confirm={pendingDelete}
            on:click={() => handleProxyContextMenuAction('delete')}
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

    .create-buttons {
        position: fixed;
        left: 24px;
        top: 80px;
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
        z-index: 10;
    }

    .create-btn {
        @apply bg-accent hover:bg-accent-hover text-text-primary;
        padding: 0.75rem 1.5rem;
        border-radius: 0.5rem;
        font-weight: 600;
        box-shadow: 0 4px 6px rgba(0, 0, 0, 0.3);
    }

    .create-btn.proxy {
        background-color: var(--bg-tertiary);
        border: 1px solid var(--border);
    }

    .create-btn.proxy:hover {
        background-color: var(--bg-secondary);
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
