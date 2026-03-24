import { writable } from 'svelte/store';

type ItemType = 'server' | 'proxy';

interface Position {
	x: number;
	y: number;
	type: ItemType;
}

interface DragPositions {
	[id: string]: Position | undefined;
}

function createDragPositionsStore() {
	const { subscribe, set, update } = writable<DragPositions>({});

	return {
		subscribe,
		updatePosition: (id: string, x: number, y: number, type: ItemType) => {
			update(positions => ({
				...positions,
				[id]: { x, y, type }
			}));
		},
		clearPosition: (id: string) => {
			update(positions => {
				const { [id]: _, ...rest } = positions;
				return rest;
			});
		},
		getPosition: (id: string): Position | undefined => {
			let position: Position | undefined;
			subscribe(positions => {
				position = positions[id];
			})();
			return position;
		}
	};
}

export const dragPositions = createDragPositionsStore();
