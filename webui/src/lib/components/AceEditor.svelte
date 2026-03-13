<script lang="ts">
	import { onMount, onDestroy, afterUpdate } from 'svelte';
	import ace from 'ace-builds/src-noconflict/ace';
	import 'ace-builds/src-noconflict/theme-terminal';
	import 'ace-builds/src-noconflict/mode-json';
	import 'ace-builds/src-noconflict/mode-yaml';
	import 'ace-builds/src-noconflict/mode-properties';
	import 'ace-builds/src-noconflict/mode-text';
	import 'ace-builds/src-noconflict/ext-searchbox';

	export let value: string = '';
	export let language: string | null = null;
	export let readonly: boolean = false;

	let editorContainer: HTMLElement;
	let editor: any;

	let isUpdatingFromAce = false;

	onMount(() => {
		ace.config.set('basePath', '/node_modules/ace-builds/src-noconflict/');

		editor = ace.edit(editorContainer);

		editor.setTheme('ace/theme/terminal');

		if (language) {
			editor.session.setMode(`ace/mode/${language}`);
		} else {
			editor.session.setMode('ace/mode/text');
		}

		editor.setOptions({
			fontSize: '0.875rem',
			fontFamily: "'Courier New', monospace",
			showLineNumbers: true,
			showPrintMargin: false,
			tabSize: 4,
			useSoftTabs: true,
			readOnly: readonly,
			useWorker: false
		});

		editor.setValue(value, -1);
		editor.clearSelection();

		editor.session.on('change', () => {
			isUpdatingFromAce = true;
			value = editor.getValue();
		});

		const handleResize = () => editor.resize();
		window.addEventListener('resize', handleResize);

		return () => {
			window.removeEventListener('resize', handleResize);
		};
	});

	afterUpdate(() => {
		if (editor && !isUpdatingFromAce && value !== editor.getValue()) {
			const cursor = editor.getCursorPosition();
			editor.setValue(value, -1);
			editor.moveCursorToPosition(cursor);
		}
		isUpdatingFromAce = false;
	});

	onDestroy(() => {
		if (editor) {
			editor.destroy();
		}
	});
</script>

<div bind:this={editorContainer} class="ace-editor-container"></div>

<style>
	.ace-editor-container {
		position: relative;
		width: 100%;
		height: 100%;
	}
</style>
