import { writable, derived } from 'svelte/store';
import { settingsApi } from '$lib/api';

export const globalSettings = writable<{ proxy_mc_version: string }>({
	proxy_mc_version: '1.21.11'
});

export const proxyMCVersion = derived(globalSettings, ($settings) => $settings.proxy_mc_version);

export const loadGlobalSettings = async () => {
	try {
		const settings = await settingsApi.get();
		globalSettings.set({
			proxy_mc_version: settings.proxy_mc_version || '1.21.11'
		});
	} catch (err) {
		console.error('Failed to load global settings:', err);
	}
};

export const saveGlobalSettings = async (data: { proxy_mc_version: string }) => {
	try {
		await settingsApi.update(data);
		globalSettings.set({
			proxy_mc_version: data.proxy_mc_version
		});
	} catch (err) {
		console.error('Failed to save global settings:', err);
		throw err;
	}
};
