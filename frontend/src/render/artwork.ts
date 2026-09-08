/** 可复现的透明游戏素材；同时用于运行时缓存与 PNG 导出。 */
export function penguinArtwork(player: number): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 256;
  const c = canvas.getContext('2d')!;
  c.translate(128, 132);
  const orange = player === 1;
  const ellipse = (x: number, y: number, rx: number, ry: number, fill: string | CanvasGradient, angle = 0) => {
    c.beginPath();
    c.ellipse(x, y, rx, ry, angle, 0, Math.PI * 2);
    c.fillStyle = fill;
    c.fill();
  };
  const shade = (x: number, y: number, r: number, a: string, b: string) => {
    const g = c.createRadialGradient(x, y, 4, 0, 0, r);
    g.addColorStop(0, a);
    g.addColorStop(1, b);
    return g;
  };
  ellipse(0, 86, 64, 13, 'rgba(16,68,78,.18)');
  ellipse(-29, 77, 27, 12, '#C46E32', -.13);
  ellipse(29, 77, 27, 12, '#C46E32', .13);
  ellipse(-29, 73, 26, 10, '#FFBB62', -.13);
  ellipse(29, 73, 26, 10, '#FFBB62', .13);
  const body = shade(-32, -61, 130, orange ? '#FFC574' : '#617782', orange ? '#D57833' : '#202F3A');
  ellipse(-68, 9, 18, 46, body, .35);
  ellipse(68, 9, 18, 46, body, -.35);
  ellipse(0, -1, 66, 83, body);
  ellipse(-17, -64, 25, 9, orange ? 'rgba(255,230,175,.4)' : 'rgba(185,215,222,.18)', -.4);
  // 连续的白脸与腹部轮廓，让小尺寸下的角色仍然清晰。
  c.beginPath();
  c.moveTo(0, -45);
  c.bezierCurveTo(-50, -75, -51, -22, -39, -9);
  c.bezierCurveTo(-69, 54, -25, 78, 0, 73);
  c.bezierCurveTo(25, 78, 69, 54, 39, -9);
  c.bezierCurveTo(51, -22, 50, -75, 0, -45);
  c.fillStyle = shade(-15, -22, 110, '#FFFFFF', orange ? '#F7DAB4' : '#D7E7EA');
  c.fill();
  for (const x of [-23, 23]) {
    ellipse(x, -33, 7.2, 10, '#233541');
    ellipse(x - 2, -37, 2.5, 3.3, '#FFFFFF');
    ellipse(x + 1.5, -29, 1.3, 1.6, '#67838F');
    ellipse(x * 1.52, -13, 9, 4.5, orange ? '#EEAA77' : '#EDBBB0');
  }
  c.beginPath();
  c.moveTo(-12, -22);
  c.quadraticCurveTo(0, -28, 12, -22);
  c.quadraticCurveTo(8, -9, 0, -6);
  c.quadraticCurveTo(-8, -9, -12, -22);
  c.fillStyle = '#E9993F';
  c.fill();
  c.beginPath(); c.moveTo(-9, -21); c.lineTo(8, -21);
  c.strokeStyle = '#FFD58B'; c.lineWidth = 3; c.stroke();
  // 围巾是额外的队伍标识，避免只靠身体明暗区分。
  c.beginPath();
  c.moveTo(-46, 0); c.quadraticCurveTo(0, 18, 46, 0);
  c.lineTo(43, 13); c.quadraticCurveTo(0, 30, -43, 13); c.closePath();
  c.fillStyle = orange ? '#C45E60' : '#278F91'; c.fill();
  c.beginPath(); c.moveTo(25, 15); c.lineTo(42, 10); c.lineTo(49, 41); c.lineTo(32, 37); c.closePath();
  c.fillStyle = orange ? '#E5807A' : '#51B7B0'; c.fill();
  c.beginPath(); c.moveTo(-39, 5); c.quadraticCurveTo(-8, 16, 19, 11);
  c.strokeStyle = orange ? '#F8AD98' : '#91DAD0'; c.lineWidth = 2; c.stroke();
  return canvas;
}

export function iceArtwork(): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 256;
  const c = canvas.getContext('2d')!;
  const polygon = (cy: number, r: number) => {
    c.beginPath();
    for (let i = 0; i < 6; i++) {
      const angle = (i * 60 - 30) * Math.PI / 180;
      const x = 128 + Math.cos(angle) * r;
      const y = cy + Math.sin(angle) * r;
      if (i === 0) c.moveTo(x, y); else c.lineTo(x, y);
    }
    c.closePath();
  };
  c.shadowColor = 'rgba(12,65,78,.24)'; c.shadowBlur = 8; c.shadowOffsetY = 4;
  polygon(135, 109);
  const side = c.createLinearGradient(40, 0, 220, 250);
  side.addColorStop(0, '#A6E0E6'); side.addColorStop(.55, '#65B7C9'); side.addColorStop(1, '#368BA4');
  c.fillStyle = side; c.fill(); c.shadowColor = 'transparent';
  polygon(122, 109);
  const top = c.createLinearGradient(55, 20, 200, 225);
  top.addColorStop(0, '#FFFFFF'); top.addColorStop(.48, '#E8F8FA'); top.addColorStop(1, '#B9E4EC');
  c.fillStyle = top; c.fill(); c.strokeStyle = '#F8FFFF'; c.lineWidth = 3; c.stroke();
  c.save(); c.clip();
  c.beginPath(); c.moveTo(20, 117); c.lineTo(152, 8); c.lineTo(190, 8); c.lineTo(42, 160); c.closePath();
  c.fillStyle = 'rgba(255,255,255,.48)'; c.fill();
  c.beginPath(); c.moveTo(170, 214); c.lineTo(193, 181); c.lineTo(185, 164);
  c.moveTo(193, 181); c.lineTo(209, 177);
  c.moveTo(42, 81); c.lineTo(62, 92); c.lineTo(69, 108);
  c.strokeStyle = 'rgba(107,180,198,.35)'; c.lineWidth = 1.6; c.stroke();
  for (let i = 0; i < 44; i++) {
    const x = 32 + ((i * 73) % 191), y = 20 + ((i * 43) % 206);
    c.beginPath(); c.arc(x, y, i % 3 === 0 ? 1.6 : .75, 0, Math.PI * 2);
    c.fillStyle = 'rgba(255,255,255,.65)'; c.fill();
  }
  c.restore();
  return canvas;
}
