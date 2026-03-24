import { writable, derived } from 'svelte/store';
import { settingsApi, type Backup } from '$lib/api';

export interface GlobalSettings {
	proxy_mc_version: string;
	backup_time: string;
	backup_interval_days: string;
	retention_count: number;
	server_timezone: string;
	background_texture_scale: number;
}

export const globalSettings = writable<GlobalSettings>({
	proxy_mc_version: '1.21.11',
	backup_time: '03:00',
	backup_interval_days: '3',
	retention_count: 2,
	server_timezone: 'UTC',
	background_texture_scale: 4
});

export const backgroundTextureUrl = writable<string | null>(null);

export const proxyMCVersion = derived(globalSettings, ($settings) => $settings.proxy_mc_version);

export const backupStatus = writable<'idle' | 'in_progress' | 'complete' | 'failed'>('idle');

export const loadGlobalSettings = async () => {
	try {
		const settings = await settingsApi.get();
		globalSettings.set({
			proxy_mc_version: settings.proxy_mc_version || '1.21.11',
			backup_time: settings.backup_time || '03:00',
			backup_interval_days: settings.backup_interval_days || '3',
			retention_count: settings.retention_count ? parseInt(settings.retention_count) : 2,
			server_timezone: settings.server_timezone || 'UTC',
			background_texture_scale: settings.background_texture_scale ? parseInt(settings.background_texture_scale) : 4
		});
	} catch (err) {
		console.error('Failed to load global settings:', err);
	}
};

export const saveGlobalSettings = async (data: GlobalSettings) => {
	try {
		await settingsApi.update(data);
		globalSettings.set(data);
	} catch (err) {
		console.error('Failed to save global settings:', err);
		throw err;
	}
};

export const loadBackgroundTexture = async () => {
	try {
		const url = await settingsApi.getBackgroundTexture();
		backgroundTextureUrl.set(url);
	} catch (err) {
		backgroundTextureUrl.set(null);
	}
};

