/** 六边形数学：axial 坐标 + pointy-top 网格 */

/** 六个相邻方向（axial 坐标偏移） */
export const HEX_DIRS: ReadonlyArray<readonly [number, number]> = [
  [1, 0],
  [1, -1],
  [0, -1],
  [-1, 0],
  [-1, 1],
  [0, 1],
];

/**
 * 生成官方 60 格棋盘：8 行，偶数行 8 格、奇数行 7 格。
 * 返回每格的 axial 坐标（按行序展开成一维数组，索引即格号）。
 */
export function boardCoords(): Array<{ q: number; r: number }> {
  const cells: Array<{ q: number; r: number }> = [];
  for (let row = 0; row < 8; row++) {
    const cols = row % 2 === 0 ? 8 : 7;
    for (let col = 0; col < cols; col++) {
      // odd-r offset → axial
      const q = col - (row - (row & 1)) / 2;
      const r = row;
      cells.push({ q, r });
    }
  }
  return cells;
}

/** 格索引 → 像素中心（pointy-top，size 为外接圆半径） */
export function axialToPixel(q: number, r: number, size: number): { x: number; y: number } {
  return {
    x: size * Math.sqrt(3) * (q + r / 2),
    y: size * 1.5 * r,
  };
}

/** pointy-top 六边形顶点（用于渲染） */
export function hexCorners(cx: number, cy: number, size: number): Array<[number, number]> {
  const pts: Array<[number, number]> = [];
  for (let i = 0; i < 6; i++) {
    const a = (Math.PI / 180) * (60 * i - 30);
    pts.push([cx + size * Math.cos(a), cy + size * Math.sin(a)]);
  }
  return pts;
}
