<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { browser } from '$app/environment';
	import { api, type AuthStatus } from '$lib/api';
	import { token } from '$lib/stores/auth';

	if (browser) {
		const saved = localStorage.getItem('accent_color');
		if (saved) {
			document.documentElement.style.setProperty('--accent', saved);
			const r = parseInt(saved.slice(1, 3), 16);
			const g = parseInt(saved.slice(3, 5), 16);
			const b = parseInt(saved.slice(5, 7), 16);
			const dr = Math.max(0, Math.round(r * 0.75)).toString(16).padStart(2, '0');
			const dg = Math.max(0, Math.round(g * 0.75)).toString(16).padStart(2, '0');
			const db = Math.max(0, Math.round(b * 0.75)).toString(16).padStart(2, '0');
			document.documentElement.style.setProperty('--accent-hover', `#${dr}${dg}${db}`);
		}
	}

	let authStatus: AuthStatus | null = null;
	let username = '';
	let password = '';
	let error = '';
	let loading = true;

	onMount(async () => {
		try {
			authStatus = await api.get<AuthStatus>('/api/auth/status');
			if (authStatus.logged_in) {
				goto('/');
			}
		} catch (err) {
			console.error('Failed to check auth status:', err);
		} finally {
			loading = false;
		}
	});

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		
		try {
			if (!authStatus?.setup_complete) {
				const response = await api.post<{ success: boolean }>('/api/auth/setup', {
					username,
					password
				});
			} else {
				await login();
			}
		} catch (err) {
			console.error('Setup/Login failed:', err);
			error = err instanceof Error ? err.message : 'An error occurred';
		}
	}

	async function login() {
		const response = await api.post<{ token: string; expires_at: string }>('/api/auth/login', {
			username,
			password
		});
		token.set(response.token);
		goto('/');
	}
</script>

<svelte:head>
	<title>Login - DemiMine</title>
</svelte:head>

<main class="login-container">
	{#if loading}
		<p class="loading">Loading...</p>
	{:else if authStatus}
		<form class="login-form" on:submit={handleSubmit}>
			{#if !authStatus.setup_complete}
				<h1>Welcome to DemiMine</h1>
				<p>Create your admin account to get started</p>
			{:else}
				<h1>Login</h1>
				<p>Enter your credentials to continue</p>
			{/if}
			
			{#if error}
				<p class="error">{error}</p>
			{/if}
			
			<div class="form-group">
				<label for="username">Username</label>
				<input 
					type="text" 
					id="username"
					bind:value={username}
					required
				/>
			</div>
			
			<div class="form-group">
				<label for="password">Password</label>
				<input 
					type="password" 
					id="password"
					bind:value={password}
					required
				/>
			</div>
			
			<button type="submit" disabled={!username || !password}>
				{#if !authStatus.setup_complete}
					Create Account
				{:else}
					Login
				{/if}
			</button>
		</form>
	{/if}
</main>

<style>
	.login-container {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 100vh;
		padding-top: 56px;
		background-color: var(--bg-primary);
	}

	.login-form {
		background-color: var(--bg-secondary);
		border: 3px solid var(--border);
		border-radius: 0;
		padding: 2rem;
		width: 100%;
		max-width: 400px;
	}

	h1 {
		color: var(--text-primary);
		margin-top: 0;
		margin-bottom: 0.5rem;
	}

	p {
		color: var(--text-secondary);
		margin-bottom: 1.5rem;
	}

	.form-group {
		margin-bottom: 1rem;
	}

	label {
		color: var(--text-secondary);
		display: block;
		margin-bottom: 0.5rem;
	}

	input {
		width: 100%;
		background-color: var(--bg-primary);
		border: 3px solid var(--border);
		color: var(--text-primary);
		padding: 0.75rem;
		border-radius: 0;
	}

	input:focus {
		outline: none;
		border-color: var(--accent);
	}

	.error {
		color: var(--error);
		margin-bottom: 1rem;
	}

	button[type="submit"] {
		width: 100%;
		background-color: var(--accent);
		color: var(--text-primary);
		border: none;
		padding: 0.75rem 1.5rem;
		border-radius: 0;
		font-weight: 600;
		cursor: pointer;
		margin-top: 0.5rem;
	}

	button[type="submit"]:hover:not(:disabled) {
		background-color: var(--accent-hover);
	}

	button[type="submit"]:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.loading {
		color: var(--text-secondary);
		font-size: 1.125rem;
	}
</style>
