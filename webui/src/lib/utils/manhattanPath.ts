export interface Point {
	x: number;
	y: number;
}

export function generateManhattanPath(
	x1: number,
	y1: number,
	x2: number,
	y2: number
): Point[] {
	if (y1 === y2) {
		return [{ x: x1, y: y1 }, { x: x2, y: y2 }];
	}

	if (x1 === x2) {
		return [{ x: x1, y: y1 }, { x: x2, y: y2 }];
	}

	const dx = x2 - x1;
	const dy = y2 - y1;
	const distance = Math.sqrt(dx * dx + dy * dy);
	const stepCount = Math.max(2, Math.min(3, Math.floor(distance / 150)));

	const primaryHorizontal = Math.abs(dx) > Math.abs(dy);
	const points: Point[] = [{ x: x1, y: y1 }];

	for (let i = 1; i <= stepCount; i++) {
		if (primaryHorizontal) {
			const hX = x1 + (dx * i / stepCount);
			points.push({ x: hX, y: points[points.length - 1].y });

			if (i < stepCount) {
				const vY = y1 + (dy * i / stepCount);
				points.push({ x: hX, y: vY });
			}
		} else {
			const vY = y1 + (dy * i / stepCount);
			points.push({ x: points[points.length - 1].x, y: vY });

			if (i < stepCount) {
				const hX = x1 + (dx * i / stepCount);
				points.push({ x: hX, y: vY });
			}
		}
	}

	points.push({ x: x2, y: y2 });

	return points;
}

export function pointsToPolylineString(points: Point[]): string {
	return points.map(p => `${p.x},${p.y}`).join(' ');
}
