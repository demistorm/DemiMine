import { writable, derived } from 'svelte/store';
import type { Server, Proxy } from '$lib/api';
import { api } from '$lib/api';

export const servers = writable<Server[]>([]);
export const proxies = writable<Proxy[]>([]);
export const isAuthenticated = writable<boolean>(false);
export const canvasZoom = writable<number>(1);
export const canvasPan = writable<{ x: number; y: number }>({ x: 0, y: 0 });

export interface BackupStatus {
	status: 'idle' | 'pending' | 'stopping_servers' | 'stopping_proxies' |
		'archiving' | 'restarting' | 'complete' | 'failed' |
		'restoring_extracting' | 'restoring_applying' | 'restoring_complete';
	backupId: number | null;
	message: string;
	timestamp: number | null;
}

export const backupStatus = writable<BackupStatus>({
	status: 'idle',
	backupId: null,
	message: '',
	timestamp: null
});

export const isBackupRunning = derived(backupStatus, $s =>
	$s.status !== 'idle' && $s.status !== 'complete' && $s.status !== 'failed' && $s.status !== 'restoring_complete'
);

export const isRestoreRunning = derived(backupStatus, $s =>
	$s.status.startsWith('restoring')
);

export const backupOperationLabel = derived(backupStatus, $s => {
	switch ($s.status) {
		case 'pending': return 'Starting...';
		case 'stopping_servers': return 'Stopping servers...';
		case 'stopping_proxies': return 'Stopping proxies...';
		case 'archiving': return 'Creating archive...';
		case 'restarting': return 'Restarting...';
		case 'restoring_extracting': return 'Extracting...';
		case 'restoring_applying': return 'Applying...';
		case 'complete': return 'Complete!';
		case 'restoring_complete': return 'Restarting...';
		case 'failed': return 'Failed';
		default: return '';
	}
});

export async function loadServers() {
	try {
		const data = await api.get<Server[]>('/api/servers');
		servers.set(data);
	} catch (error) {
		console.error('Failed to load servers:', error);
	}
}

export async function loadProxies() {
	try {
		const data = await api.get<Proxy[]>('/api/proxies');
		proxies.set(data);
	} catch (error) {
		console.error('Failed to load proxies:', error);
	}
}

export async function updateServerPosition(serverId: number, x: number, y: number) {
	try {
		await api.patch(`/api/servers/${serverId}`, { canvas_x: x, canvas_y: y });
		servers.update(serversList =>
			serversList.map(s =>
				s.id === serverId ? { ...s, canvas_x: x, canvas_y: y } : s
			)
		);
	} catch (error) {
		console.error('Failed to update server position:', error);
	}
}

export function updateServerStatus(serverId: number, status: string, playerCount?: number) {
	servers.update(serversList =>
		serversList.map(s => 
			s.id === serverId ? { 
				...s, 
				status,
				player_count: playerCount !== undefined ? playerCount : s.player_count
			} : s
		)
	);
}

export function deleteServerFromStore(serverId: number) {
	servers.update(serversList => serversList.filter(s => s.id !== serverId));
}

export async function updateProxyPosition(proxyId: number, x: number, y: number) {
	try {
		await api.patch(`/api/proxies/${proxyId}`, { canvas_x: x, canvas_y: y });
		proxies.update(proxiesList =>
			proxiesList.map(p =>
				p.id === proxyId ? { ...p, canvas_x: x, canvas_y: y } : p
			)
		);
	} catch (error) {
		console.error('Failed to update proxy position:', error);
	}
}

export function updateProxyStatus(proxyId: number, status: string, playerCount?: number) {
	proxies.update(proxiesList =>
		proxiesList.map(p => 
			p.id === proxyId ? { 
				...p, 
				status,
				player_count: playerCount !== undefined ? playerCount : p.player_count
			} : p
		)
	);
}

export function deleteProxyFromStore(proxyId: number) {
	proxies.update(proxiesList => proxiesList.filter(p => p.id !== proxyId));
}
