<script lang="ts">
	import '../app.css';
	import { page } from '$app/stores';
	import { token } from '$lib/stores/auth';

	function handleLogout() {
		token.set(null);
	}
</script>

{#if $page.url.pathname !== '/login'}
	<nav class="top-nav">
		<div class="nav-brand">
			<a href="/" class="nav-link">
				<strong>DemiMine</strong>
			</a>
		</div>
		
		<div class="nav-links">
			<a href="/" class="nav-link" class:active={$page.url.pathname === '/'}>
				Servers
			</a>
			{#if $token}
				<button on:click={handleLogout} class="nav-link">
					Logout
				</button>
			{:else}
				<a href="/login" class="nav-link">
					Login
				</a>
			{/if}
		</div>
	</nav>
{/if}

<slot />

<style>
	.top-nav {
		background-color: var(--bg-secondary);
		border-bottom: 1px solid var(--border);
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 0.75rem 1.5rem;
		height: 56px;
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 100;
	}

	.nav-brand {
		display: flex;
		align-items: center;
	}

	.nav-link {
		color: var(--text-primary);
		text-decoration: none;
		padding: 0.5rem 1rem;
		border-radius: 0.25rem;
		transition: color 0.2s;
		cursor: pointer;
		background: none;
		border: none;
		font-size: 1rem;
	}

	.nav-link:hover {
		color: var(--accent);
	}

	.nav-link.active {
		color: var(--accent);
		font-weight: 600;
	}

	.nav-links {
		display: flex;
		gap: 0.5rem;
		align-items: center;
	}
</style>
